package query

type TransactionFilters struct {
	DateFrom       string
	DateTo         string
	AmountMin      *float64
	AmountMax      *float64
	LastUpdated    string
	ExcludeDeleted bool
}

type SortOption struct {
	Field     string
	Direction string
}
