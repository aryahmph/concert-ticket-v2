package vars

import (
	"concert-ticket/model"
	"sync/atomic"
	"unsafe"
)

// categoryDataPtr holds a pointer to the current slice of category data.
// This approach allows for lock-free reads with atomic updates.
var categoryDataPtr unsafe.Pointer

// GetCategories returns the current category data.
// This operation is lock-free and safe for concurrent access.
func GetCategories() []model.CategoryResponse {
	ptr := atomic.LoadPointer(&categoryDataPtr)
	if ptr == nil {
		return nil
	}
	return *(*[]model.CategoryResponse)(ptr)
}

// SetCategories atomically updates the category data.
// It creates a copy of the input data to ensure consistency.
// Pass nil or empty slice to clear categories.
func SetCategories(categories []model.CategoryResponse) {
	var ptr unsafe.Pointer

	if len(categories) > 0 {
		// Only create a copy if we have data
		categoriesCopy := make([]model.CategoryResponse, len(categories))
		copy(categoriesCopy, categories)
		ptr = unsafe.Pointer(&categoriesCopy)
	}

	// Atomically replace the pointer
	atomic.StorePointer(&categoryDataPtr, ptr)
}

// Initialize with nil pointer (will return nil slice from GetCategories)
func init() {
	atomic.StorePointer(&categoryDataPtr, nil)
}
