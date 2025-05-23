// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.29.0
// source: categories.sql

package sqlgen

import (
	"context"
)

const bulkIncrementCategoryQuantity = `-- name: BulkIncrementCategoryQuantity :exec
UPDATE categories
SET quantity = quantity + CASE
                              WHEN id = 1 THEN $1::integer
                              WHEN id = 2 THEN $2::integer
                              WHEN id = 3 THEN $3::integer
                              WHEN id = 4 THEN $4::integer
                              WHEN id = 5 THEN $5::integer
                              WHEN id = 6 THEN $6::integer
                              WHEN id = 7 THEN $7::integer
                              WHEN id = 8 THEN $8::integer
                              WHEN id = 9 THEN $9::integer
    END
WHERE id IN (1, 2, 3, 4, 5, 6, 7, 8, 9)
`

type BulkIncrementCategoryQuantityParams struct {
	Column1 int32
	Column2 int32
	Column3 int32
	Column4 int32
	Column5 int32
	Column6 int32
	Column7 int32
	Column8 int32
	Column9 int32
}

func (q *Queries) BulkIncrementCategoryQuantity(ctx context.Context, arg BulkIncrementCategoryQuantityParams) error {
	_, err := q.db.Exec(ctx, bulkIncrementCategoryQuantity,
		arg.Column1,
		arg.Column2,
		arg.Column3,
		arg.Column4,
		arg.Column5,
		arg.Column6,
		arg.Column7,
		arg.Column8,
		arg.Column9,
	)
	return err
}

const findAllCategories = `-- name: FindAllCategories :many
SELECT id, name, price, max_row, max_col, quantity
FROM categories
`

type FindAllCategoriesRow struct {
	ID       int16
	Name     string
	Price    int32
	MaxRow   int32
	MaxCol   int32
	Quantity int32
}

func (q *Queries) FindAllCategories(ctx context.Context) ([]FindAllCategoriesRow, error) {
	rows, err := q.db.Query(ctx, findAllCategories)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []FindAllCategoriesRow
	for rows.Next() {
		var i FindAllCategoriesRow
		if err := rows.Scan(
			&i.ID,
			&i.Name,
			&i.Price,
			&i.MaxRow,
			&i.MaxCol,
			&i.Quantity,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
