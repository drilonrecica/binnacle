// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	authpkg "github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/metrics"
)

type Authorizer interface{ Authorize(*http.Request) bool }

// Rechecker re-validates an open stream's credentials on every heartbeat
// without counting the check as activity (SEC-12). Authorizers that lack it are
// re-checked with Authorize instead.
type Rechecker interface{ Recheck(*http.Request) bool }

type DemoAuthorizer bool

func (a DemoAuthorizer) Authorize(*http.Request) bool { return bool(a) }

// streamHeartbeat is how often an open stream sends a comment frame and
// re-checks its session. Tests shorten it.
var streamHeartbeat = 20 * time.Second

func recheckFunc(auth Authorizer, r *http.Request) func() bool {
	if rc, ok := auth.(Rechecker); ok {
		return func() bool { return rc.Recheck(r) }
	}
	return func() bool { return auth.Authorize(r) }
}

// EnableLive serves the live stream. Every stream shares one engine
// subscription through a fan-out hub (RUN-5); opts sets the snapshot interval
// and the concurrent-client cap.
func (s *Server) EnableLive(engine *metrics.Engine, auth Authorizer, protection *authpkg.Protection, opts LiveOptions, decorators ...SnapshotDecorator) {
	hub := newLiveHub(engine, opts, decorators)
	s.live = hub
	s.Handle("/api/v1/live", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, 405, Error{Code: "method_not_allowed", Message: "Only GET is supported."})
			return
		}
		if auth == nil || !auth.Authorize(r) {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		if ok, retry := protection.AllowLive(r); !ok {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", maxRetry(retry)))
			WriteError(w, 429, Error{Code: "rate_limited", Message: "Too many live connections. Try again shortly.", Details: map[string]int{"retryAfterSeconds": maxRetry(retry)}})
			return
		}
		if engine == nil {
			WriteError(w, 503, Error{Code: "live_unavailable", Message: "Live updates are unavailable."})
			return
		}
		stream(w, r, hub, recheckFunc(auth, r))
	}))
	s.Handle("/api/v1/session", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth == nil || !auth.Authorize(r) {
			WriteError(w, 401, Error{Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// LiveClients is the number of open live streams.
func (s *Server) LiveClients() int { return s.live.Clients() }

// liveCapacityRetry is the Retry-After sent when every live slot is taken.
const liveCapacityRetry = 10

func stream(w http.ResponseWriter, r *http.Request, hub *liveHub, recheck func() bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, 500, Error{Code: "stream_unsupported", Message: "Streaming is unavailable."})
		return
	}
	client, ok := hub.register()
	if !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", liveCapacityRetry))
		WriteError(w, 503, Error{Code: "live_capacity", Message: fmt.Sprintf("All %d live connections are in use. Close another Binnacle tab or raise live.max_clients.", hub.opts.MaxClients), Details: map[string]int{"maxClients": hub.opts.MaxClients, "retryAfterSeconds": liveCapacityRetry}})
		return
	}
	defer hub.unregister(client)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	// Send the headers now so the client sees the stream open even before
	// the first snapshot.
	w.WriteHeader(http.StatusOK)
	f.Flush()
	// cursor is the highest id written; frames never move it backwards, and
	// events at or below it (already replayed) are skipped.
	var cursor metrics.Sequence
	// Registering before reading the ring means an event published meanwhile
	// is either replayed here or queued for the client, never missed.
	if n, err := strconv.ParseUint(r.Header.Get("Last-Event-ID"), 10, 64); err == nil {
		// The cursor starts from what is replayed, not from the header: after
		// a restart the sequence begins again below it.
		for _, event := range hub.engine.EventsAfter(metrics.Sequence(n)) {
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			writeFrame(w, liveFrame{kind: "event", id: event.Seq, data: data})
			cursor = event.Seq
		}
		f.Flush()
	}
	beat := time.NewTicker(streamHeartbeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			if !recheck() {
				// The session ended (logout, MFA change, expiry): close so the
				// client re-authenticates instead of reading forever.
				return
			}
			fmt.Fprint(w, ": heartbeat\n\n")
			f.Flush()
		case <-client.notify:
			frames, closed := client.take()
			if closed {
				// Too far behind or the hub ended: close so the client resumes
				// with Last-Event-ID instead of silently missing events.
				return
			}
			for _, frame := range frames {
				if frame.kind == "event" && frame.id <= cursor {
					continue
				}
				cursor = max(cursor, frame.id)
				frame.id = cursor
				writeFrame(w, frame)
			}
			f.Flush()
		}
	}
}

func writeFrame(w http.ResponseWriter, frame liveFrame) {
	fmt.Fprintf(w, "event: %s\nid: %d\ndata: ", frame.kind, frame.id)
	_, _ = w.Write(frame.data)
	fmt.Fprint(w, "\n\n")
}
