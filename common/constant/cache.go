package constant

import "time"

const (
	EachCategoryQuantityKey = "category:%d:quantity"
	OrderEmailLock          = "order:email_lock:%s"
)

const (
	OrderEmailLockDefaultTTL = 1 * time.Minute
)
