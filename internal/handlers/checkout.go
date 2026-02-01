package handlers

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
)

// CheckoutRequest represents the expected JSON body for /checkout
// (e.g., { "user_id": 123 })
type CheckoutRequest struct {
	UserID int64 `json:"user_id"`
}

// CheckoutHandler handles POST /checkout requests.
func CheckoutHandler(spannerClient *spanner.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		rand.Seed(time.Now().UnixNano())
		var req CheckoutRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		ctx := c.Request.Context()
		_, err := spannerClient.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			// Generate unique IDs for Order and Payment
			orderID := rand.Int63()
			paymentID := rand.Int63()
			// Read all items from ShoppingCarts for the user
			// Use a key range to read all ShoppingCarts rows for the user
			cartIter := txn.Read(
				ctx,
				"ShoppingCarts",
				spanner.KeyRange{
					Start: spanner.Key{req.UserID},
					End:   spanner.Key{req.UserID + 1},
					Kind:  spanner.ClosedOpen,
				},
				[]string{"ProductID", "Quantity"},
			)
			defer cartIter.Stop()

			var cartItems []struct {
				ProductID int64
				Quantity  int64
			}
			for {
				row, err := cartIter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return err
				}
				var item struct {
					ProductID int64
					Quantity  int64
				}
				if err := row.Columns(&item.ProductID, &item.Quantity); err != nil {
					return err
				}
				cartItems = append(cartItems, item)
			}

			// Fetch user email for debug
			var userEmail string
			userRow, err := txn.ReadRow(ctx, "Users", spanner.Key{req.UserID}, []string{"Email"})
			if err == nil {
				_ = userRow.Columns(&userEmail)
			}

			// Debug: log cart items and user email
			fmt.Printf("[DEBUG] UserID: %d, Email: %s, CartItems: %v\n", req.UserID, userEmail, cartItems)

			if len(cartItems) == 0 {
				fmt.Printf("[DEBUG] Cart is empty for user %d (email: %s)\n", req.UserID, userEmail)
				return fmt.Errorf("cart is empty")
			}

			// Calculate total price by fetching product prices
			var total float64
			orderItems := make([]*spanner.Mutation, 0, len(cartItems))
			for i, item := range cartItems {
				stmt := spanner.Statement{
					SQL:    "SELECT PriceUSD FROM Products WHERE ProductID = @pid",
					Params: map[string]interface{}{"pid": item.ProductID},
				}
				row, err := txn.Query(ctx, stmt).Next()
				if err != nil {
					return err
				}
				var price spanner.NullNumeric
				if err := row.Columns(&price); err != nil {
					return err
				}
				if !price.Valid {
					return fmt.Errorf("price is NULL for product %d", item.ProductID)
				}
				priceFloat, _ := price.Numeric.Float64()
				total += priceFloat * float64(item.Quantity)
				orderItems = append(orderItems, spanner.Insert(
					"OrderItems",
					[]string{"OrderID", "OrderItemID", "ProductID", "Quantity", "PriceAtOrderUSD"},
					[]interface{}{orderID, int64(i + 1), item.ProductID, item.Quantity, price},
				))
			}

			// Insert into Orders
			orderMutation := spanner.Insert(
				"Orders",
				[]string{"OrderID", "UserID", "OrderDate", "TotalAmountUSD", "OrderStatus"},
				[]interface{}{orderID, req.UserID, spanner.CommitTimestamp, fmt.Sprintf("%f", total), "PENDING"},
			)
			if err := txn.BufferWrite([]*spanner.Mutation{orderMutation}); err != nil {
				return err
			}

			// Insert OrderItems
			if err := txn.BufferWrite(orderItems); err != nil {
				return err
			}

			// Insert Payment
			paymentMutation := spanner.Insert(
				"Payments",
				[]string{"PaymentID", "OrderID", "UserID", "AmountUSD", "Status"},
				[]interface{}{paymentID, orderID, req.UserID, fmt.Sprintf("%f", total), "INITIATED"},
			)
			if err := txn.BufferWrite([]*spanner.Mutation{paymentMutation}); err != nil {
				return err
			}

			// Delete ShoppingCarts rows for the user
			for _, item := range cartItems {
				m := spanner.Delete("ShoppingCarts", spanner.Key{req.UserID, item.ProductID})
				if err := txn.BufferWrite([]*spanner.Mutation{m}); err != nil {
					return err
				}
			}

			return nil // commit
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "checkout successful"})
	}
}
