INSERT INTO categories (id, name, price, quantity, max_row, max_col)
VALUES (1, 'Ultimate Experience', 11000000, 500, 50, 10),
       (2, 'My Universe', 7500000, 1000, 50, 20),
       (3, 'CAT 1', 5800000, 3000, 100, 30),
       (4, 'CAT 2', 5200000, 4000, 100, 40),
       (5, 'CAT 3', 4600000, 5000, 100, 50),
       (6, 'CAT 4', 3800000, 6000, 100, 60),
       (7, 'CAT 5', 3000000, 7000, 100, 70),
       (8, 'CAT 6', 1500000, 10000, 100, 100),
       (9, 'Festival', 2500000, 15000, 150, 100);

INSERT INTO category_quantities (category_id, row, col)
SELECT c.id      AS category_id,
       row_num   AS row,
       c.max_col AS col
FROM categories c,
     GENERATE_SERIES(1, c.max_row + 1) AS row_num;