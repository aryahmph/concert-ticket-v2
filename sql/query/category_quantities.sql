-- name: DecrementCategoryQuantityCol :one
WITH selected_quantity AS (SELECT row, col
                           FROM category_quantities
                           WHERE category_quantities.category_id = $1
                             AND col > 1
                           LIMIT 1),
     updated_quantity AS (
         UPDATE
             category_quantities
                 SET col = col - 1
                 WHERE category_quantities.category_id = $1 AND col > 1
                     AND category_quantities.row = (SELECT row FROM selected_quantity)
                 RETURNING row, col)
SELECT row, col
FROM updated_quantity;