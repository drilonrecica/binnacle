// SPDX-License-Identifier: AGPL-3.0-only
package rollup

import "time"

func Bucket(t time.Time, size time.Duration) time.Time { return t.UTC().Truncate(size) }
