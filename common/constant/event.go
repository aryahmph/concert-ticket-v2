package constant

const (
	QueueStreamName = "concert_ticket_queue_stream"
)

const (
	AllWildcard      = "events.>"
	OrderWildcard    = "events.order.>"
	CategoryWildcard = "events.category.>"
	EmailWildcard    = "events.email.>"

	SubjectCreateOrder                   = "events.order.create"
	SubjectIncrementCategoryQuantity     = "events.category.increment_quantity"
	SubjectBulkIncrementCategoryQuantity = "events.category.bulk_increment_quantity"
	SubjectCallbackPayment               = "events.order.complete"
	SubjectAssignOrderTicketRowCol       = "events.order.assign_ticket_row_col"
	SubjectSendEmail                     = "events.email.send"
)
