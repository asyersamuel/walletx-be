package jobs

import "github.com/google/uuid"

func strPtr(value string) *string    { return &value }
func uuidPtr(v uuid.UUID) *uuid.UUID { return &v }
