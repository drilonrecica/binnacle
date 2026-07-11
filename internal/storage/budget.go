// SPDX-License-Identifier: AGPL-3.0-only
package storage

type BudgetState string

const (
	BudgetOK        BudgetState = "ok"
	BudgetWarning   BudgetState = "warning"
	BudgetCritical  BudgetState = "critical"
	BudgetEmergency BudgetState = "emergency"
)

func EvaluateBudget(used, target int64, warning, critical, emergency float64) BudgetState {
	if target <= 0 {
		return BudgetOK
	}
	r := float64(used) / float64(target)
	if r >= emergency {
		return BudgetEmergency
	}
	if r >= critical {
		return BudgetCritical
	}
	if r >= warning {
		return BudgetWarning
	}
	return BudgetOK
}
