-- name: InsertOrder :one
INSERT INTO orders(category_id, external_id, name, email, phone, payment_code, expired_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: FindOrderByEmailAndStatusPending :one
SELECT EXISTS (SELECT 1
               FROM orders
               WHERE email = $1
                 AND status = 'pending') AS "exists";

-- name: FindOrderByExternalIdAndStatusPending :one
SELECT id,
       category_id,
       external_id,
       name,
       email,
       phone,
       payment_code,
       expired_at
FROM orders
WHERE external_id = $1
  AND status = 'pending';

-- name: UpdateOrderStatusToCompleted :execresult
UPDATE orders
SET status     = 'completed',
    updated_at = NOW()
WHERE id = $1
  AND status = 'pending';

-- name: UpdateOrderTicketRowCol :execresult
UPDATE orders
SET ticket_row = $1,
    ticket_col = $2
WHERE id = $3
  AND status = 'completed';

-- name: BulkCancelOrders :many
UPDATE orders
SET status     = 'cancelled',
    updated_at = NOW()
WHERE id IN (SELECT id
             FROM orders
             WHERE status = 'pending'
               AND expired_at < NOW()
             LIMIT $1)
RETURNING id, category_id, name, email;