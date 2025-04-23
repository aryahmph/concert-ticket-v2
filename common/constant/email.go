package constant

const EmailOrderConfirmationTemplate = `
Dear %s,

Thank you for ordering tickets for our concert! Your order has been successfully created.

Order Details:
------------------------------------------
Order ID: %s
Ticket Category: %s
Total Amount: %s
Payment Code: %s
------------------------------------------

Please complete your payment before: %s

Payment Instructions:
1. Use the payment code above at any supported payment channel
2. Complete the payment within the time limit to secure your tickets
3. You will receive a confirmation email once payment is processed

If you have any questions or need assistance, please contact our support team at support@concert-ticket.com or call +62 812 3456 7890.

Best regards,
Concert Ticket Team

Note: This is an automated message, please do not reply to this email.
`

const EmailOrderCompletionTemplate = `
Dear %s,

Great news! Your payment has been successfully processed and your tickets are now confirmed.

✅ ORDER COMPLETED ✅

Order Details:
------------------------------------------
Order ID: %s
Ticket Category: %s
Total Amount: %s
Seat: Row %d, Seat %d
------------------------------------------

Your e-tickets are attached to this email. Please show them at the venue entrance.

Important Information:
• Please arrive at least 30 minutes before the show
• Valid ID may be required for entry
• No refunds or exchanges are permitted

If you have any questions, please contact our support team at support@concert-ticket.com or call +62 812 3456 7890.

We look forward to seeing you at the concert!

Best regards,
Concert Ticket Team
`

const EmailOrderCancellationTemplate = `
Dear %s,

We regret to inform you that your order has been cancelled.

Order Details:
------------------------------------------
Order ID: %s
Ticket Category: %s
Total Amount: %s
------------------------------------------

If you have any questions or need assistance, please contact our support team at support@concert-ticket.com or call +62 812 3456 7890.

Best regards,
Concert Ticket Team

Note: This is an automated message, please do not reply to this email.
`
