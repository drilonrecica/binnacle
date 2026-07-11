// SPDX-License-Identifier: AGPL-3.0-only
package events

import "time"

type DeploymentConfidence string

const (
	Confirmed   DeploymentConfidence = "confirmed"
	Likely      DeploymentConfidence = "likely"
	Replacement DeploymentConfidence = "replacement"
)

type Instance struct {
	ID, ResourceID, Image string
	Started, Stopped      time.Time
}
type Deployment struct {
	OldID, NewID string
	Confidence   DeploymentConfidence
	Overlap      time.Duration
}

func Correlate(old, next Instance, window time.Duration) (Deployment, bool) {
	if old.ResourceID == "" || old.ResourceID != next.ResourceID || next.Started.IsZero() {
		return Deployment{}, false
	}
	gap := next.Started.Sub(old.Stopped)
	if old.Stopped.IsZero() {
		gap = 0
	}
	if gap < 0 {
		return Deployment{old.ID, next.ID, Likely, -gap}, true
	}
	if gap <= window {
		c := Replacement
		if old.Image != "" && old.Image != next.Image {
			c = Confirmed
		}
		return Deployment{old.ID, next.ID, c, 0}, true
	}
	return Deployment{}, false
}
