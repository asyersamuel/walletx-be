package parser

type EmailParser interface {
	Parse(rawBody string) (*ParsedTransactionData, error)
}
