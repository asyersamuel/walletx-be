package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculatePaginationMeta(t *testing.T) {
	type args struct {
		total  int
		limit  int
		offset int
	}
	tests := []struct {
		// Menguji perhitungan pagination untuk halaman pertama dengan limit default
		name string
		args args
		want PaginationMeta
	}{
		{
			// Menguji perhitungan pagination untuk halaman pertama dengan limit default
			name: "First page defaults",
			args: args{
				total:  100,
				limit:  20,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      20,
				Total:      100,
				TotalPages: 5,
			},
		},
		{
			// Menguji perhitungan pagination untuk halaman tengah
			name: "Middle page",
			args: args{
				total:  100,
				limit:  20,
				offset: 40,
			},
			want: PaginationMeta{
				Page:       3,
				Limit:      20,
				Total:      100,
				TotalPages: 5,
			},
		},
		{
			// Menguji perhitungan pagination dengan custom limit
			name: "Custom limit",
			args: args{
				total:  50,
				limit:  10,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      10,
				Total:      50,
				TotalPages: 5,
			},
		},
		{
			// Menguji perhitungan pagination ketika limit = 0 (default ke 20)
			name: "Limit is zero defaults to 20",
			args: args{
				total:  50,
				limit:  0,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      20,
				Total:      50,
				TotalPages: 3,
			},
		},
		{
			// Menguji perhitungan pagination ketika total = 0
			name: "Zero total returns single page",
			args: args{
				total:  0,
				limit:  20,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      20,
				Total:      0,
				TotalPages: 1,
			},
		},
		{
			// Menguji perhitungan pagination ketika offset > total data
			name: "Offset greater than total",
			args: args{
				total:  10,
				limit:  5,
				offset: 100,
			},
			want: PaginationMeta{
				Page:       21,
				Limit:      5,
				Total:      10,
				TotalPages: 2,
			},
		},
		{
			// Menguji perhitungan pagination ketika limit > total data
			name: "Limit greater than total",
			args: args{
				total:  5,
				limit:  20,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      20,
				Total:      5,
				TotalPages: 1,
			},
		},
		{
			// Menguji perhitungan pagination dengan offset negatif
			name: "Negative offset",
			args: args{
				total:  10,
				limit:  5,
				offset: -5,
			},
			want: PaginationMeta{
				Page:       0,
				Limit:      5,
				Total:      10,
				TotalPages: 2,
			},
		},
		{
			// Menguji perhitungan pagination untuk halaman terakhir
			name: "Last page",
			args: args{
				total:  27,
				limit:  10,
				offset: 20,
			},
			want: PaginationMeta{
				Page:       3,
				Limit:      10,
				Total:      27,
				TotalPages: 3,
			},
		},
		{
			// Menguji perhitungan pagination dengan limit 1
			name: "Minimum limit of 1",
			args: args{
				total:  100,
				limit:  1,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      1,
				Total:      100,
				TotalPages: 100,
			},
		},
		{
			// Menguji perhitungan pagination ketika total tidak habis dibagi limit
			name: "Remainder division rounds up",
			args: args{
				total:  23,
				limit:  10,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      10,
				Total:      23,
				TotalPages: 3,
			},
		},
		{
			// Menguji perhitungan pagination dengan offset = limit (halaman ke-2 dengan limit standard)
			name: "Offset equals limit for second page",
			args: args{
				total:  50,
				limit:  10,
				offset: 10,
			},
			want: PaginationMeta{
				Page:       2,
				Limit:      10,
				Total:      50,
				TotalPages: 5,
			},
		},
		{
			// Menguji perhitungan pagination dengan nilai limit sangat besar
			name: "Very large limit",
			args: args{
				total:  1000,
				limit:  1000000,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      1000000,
				Total:      1000,
				TotalPages: 1,
			},
		},
		{
			// Menguji perhitungan pagination untuk edge case offset = 0 dengan limit berbeda
			name: "Zero offset with different limit",
			args: args{
				total:  7,
				limit:  3,
				offset: 0,
			},
			want: PaginationMeta{
				Page:       1,
				Limit:      3,
				Total:      7,
				TotalPages: 3,
			},
		},
		{
			// Menguji perhitungan pagination ketika offset tepat di batas terakhir
			name: "Offset at exact last item",
			args: args{
				total:  30,
				limit:  10,
				offset: 20,
			},
			want: PaginationMeta{
				Page:       3,
				Limit:      10,
				Total:      30,
				TotalPages: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculatePaginationMeta(tt.args.total, tt.args.limit, tt.args.offset)
			assert.Equal(t, tt.want, got)
		})
	}
}
