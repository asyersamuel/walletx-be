package report

// LimitReport is the result of a real-time JOIN query that pairs each active
// category limit with the amount already spent in the current calendar month.
// It is produced by BudgetRepository.GetLimitReportByUserID and consumed by
// the Telegram service to render the /limit command reply.
type LimitReport struct {
	CategoryName string  `json:"category_name"`
	CategoryIcon string  `json:"category_icon"`
	LimitAmount  float64 `json:"limit_amount"`
	TotalSpent   float64 `json:"total_spent"`
}
