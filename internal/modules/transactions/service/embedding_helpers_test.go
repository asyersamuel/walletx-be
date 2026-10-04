package service

import domain "walletx-be/internal/modules/transactions/contracts"

func validEmbedding() []float32 {
	return make([]float32, domain.EmbeddingDimension)
}
