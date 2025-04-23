package constant

import "concert-ticket/model"

var CategoryPriceById = map[int16]int64{
	1: 11_000_000, // Ultimate Experience
	2: 7_500_000,  // My Universe
	3: 5_800_000,  // CAT 1
	4: 5_200_000,  // CAT 2
	5: 4_600_000,  // CAT 3
	6: 3_800_000,  // CAT 4
	7: 3_000_000,  // CAT 5
	8: 1_500_000,  // CAT 6
	9: 2_500_000,  // Festival
}

var CategoryNameById = map[int16]string{
	1: "Ultimate Experience",
	2: "My Universe",
	3: "CAT 1",
	4: "CAT 2",
	5: "CAT 3",
	6: "CAT 4",
	7: "CAT 5",
	8: "CAT 6",
	9: "Festival",
}

var CategoriesData = []model.CategoryResponse{
	{
		Id:    1,
		Name:  "Ultimate Experience",
		Price: 11_000_000,
	},
	{
		Id:    2,
		Name:  "My Universe",
		Price: 7_500_000,
	},
	{
		Id:    3,
		Name:  "CAT 1",
		Price: 5_800_000,
	},
	{
		Id:    4,
		Name:  "CAT 2",
		Price: 5_200_000,
	},
	{
		Id:    5,
		Name:  "CAT 3",
		Price: 4_600_000,
	},
	{
		Id:    6,
		Name:  "CAT 4",
		Price: 3_800_000,
	},
	{
		Id:    7,
		Name:  "CAT 5",
		Price: 3_000_000,
	},
	{
		Id:    8,
		Name:  "CAT 6",
		Price: 1_500_000,
	},
	{
		Id:    9,
		Name:  "Festival",
		Price: 2_500_000,
	},
}
