SET TIME ZONE 'Asia/Jakarta';

CREATE TABLE IF NOT EXISTS categories
(
    id       SMALLINT PRIMARY KEY,
    name     VARCHAR(50) NOT NULL,
    price    INT         NOT NULL,
    quantity INT         NOT NULL,
    max_row  INT         NOT NULL,
    max_col  INT         NOT NULL
);

CREATE TABLE IF NOT EXISTS category_quantities
(
    id          INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    category_id SMALLINT NOT NULL,
    row         INT      NOT NULL,
    col         INT      NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_category_quantities_row ON category_quantities (row);

CREATE TYPE order_status AS ENUM ('pending', 'completed', 'cancelled');
CREATE TABLE IF NOT EXISTS orders
(
    id           INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    category_id  SMALLINT     NOT NULL,
    external_id  VARCHAR(36)  NOT NULL,
    name         VARCHAR(100) NOT NULL,
    email        VARCHAR(255) NOT NULL,
    status       order_status DEFAULT 'pending',
    payment_code VARCHAR(50)  NOT NULL,
    expired_at   TIMESTAMP    NOT NULL,
    ticket_row   INT,
    ticket_col   INT,
    created_at   TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_order_email ON orders (email);
CREATE INDEX IF NOT EXISTS idx_order_external_id ON orders (external_id);
CREATE INDEX IF NOT EXISTS idx_order_status_expired_at_pending ON orders (status, expired_at) WHERE status = 'pending';