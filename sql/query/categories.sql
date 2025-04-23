-- name: FindAllCategories :many
SELECT id, name, price, max_row, max_col, quantity
FROM categories;

-- name: BulkIncrementCategoryQuantity :exec
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
WHERE id IN (1, 2, 3, 4, 5, 6, 7, 8, 9);