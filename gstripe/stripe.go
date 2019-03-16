package gstripe

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
	"github.com/stripe/stripe-go/order"

	"github.com/scholacantorum/gala-backend/config"
	"github.com/scholacantorum/gala-backend/model"
	"github.com/scholacantorum/gala-backend/private"
)

func init() {
	stripe.LogLevel = 1
}

// FindOrCreateCustomer finds a customer with the specified name and email
// address.  If found, it updates that customer with the supplied address, if
// non-empty, and the supplied card source if non-empty.  If no matching
// customer was found, and the supplied card source is non-empty, it creates a
// new customer with the specified name, email, address if non-empty, and card
// source.  It returns the customer ID, the description of the customer's
// default source, and the address on file for the customer.  In the case where
// no matching customer was found and no card source was provided, it simply
// returns "success" with an empty customer ID and source description, and the
// address provided to it.
func FindOrCreateCustomer(guest *model.Guest, cardSource string) (status int, errmsg string) {
	var (
		cust   *stripe.Customer
		clistp *stripe.CustomerListParams
		iter   *customer.Iter
		err    error
	)

	// Look for an existing customer first.
	clistp = new(stripe.CustomerListParams)
	clistp.Filters.AddFilter("email", "", guest.Email)
	iter = customer.List(clistp)
	for iter.Next() {
		c := iter.Customer()
		if c.Description != guest.Name || c.Email != guest.Email {
			continue
		}
		if c.Metadata["monthly-donation-amount"] != "" {
			continue
		}
		cust = c
		if cust.Shipping != nil && guest.Address == "" {
			guest.Address = cust.Shipping.Address.Line1
			guest.City = cust.Shipping.Address.City
			guest.State = cust.Shipping.Address.State
			guest.Zip = cust.Shipping.Address.PostalCode
		}

		// Update the customer with the payment source for the new
		// order.
		var cparams = new(stripe.CustomerParams)
		if (cust.Shipping == nil || cust.Shipping.Address.Line1 == "") && guest.Address != "" {
			cparams.Shipping = &stripe.CustomerShippingDetailsParams{
				Name: &guest.Name,
				Address: &stripe.AddressParams{
					Line1:      &guest.Address,
					City:       &guest.City,
					State:      &guest.State,
					PostalCode: &guest.Zip,
				},
			}
		}
		if cardSource != "" {
			cparams.SetSource(cardSource)
		}
		cparams.AddExpand("default_source")
		if cust, err = customer.Update(c.ID, cparams); err != nil {
			if serr, ok := err.(*stripe.Error); ok {
				if serr.Type == stripe.ErrorTypeCard {
					return 400, serr.Msg
				}
			}
			log.Printf("stripe update customer: %s", err)
			return 500, ""
		}
		break
	}

	// If we didn't find a match, and we don't have a card source, we have
	// nothing to do.
	if cust == nil && cardSource == "" {
		return 200, ""
	}

	// Create a new customer if none was found.
	if cust == nil {
		var cparams = stripe.CustomerParams{Description: &guest.Name, Email: &guest.Email}
		cparams.SetSource(cardSource)
		cparams.AddExpand("default_source")
		cust, err = customer.New(&cparams)
		if serr, ok := err.(*stripe.Error); ok {
			if serr.Type == stripe.ErrorTypeCard {
				return 400, serr.Msg
			}
		}
		if err != nil {
			log.Printf("stripe create customer: %s", err)
			return 500, ""
		}
	}

	// If the customer doesn't have a card, don't save their customer ID.
	// We'll get it again when they add a card.
	if cust.DefaultSource == nil {
		return 200, ""
	}

	// Return the customer ID, card number, and address.
	guest.StripeCustomer = cust.ID
	guest.StripeSource = cust.DefaultSource.ID
	guest.StripeDescription = fmt.Sprintf("%s %s",
		cust.DefaultSource.SourceObject.TypeData["brand"], cust.DefaultSource.SourceObject.TypeData["last4"])
	return 200, ""
}

// UpdateCustomer updates the customer with the specified ID to have the
// supplied name, email, address if non-empty, and card source if non-empty.
// The customer's existing address and/or card source are left unchanged if the
// corresponding parameters are empty.
func UpdateCustomer(guest *model.Guest, cardSource string) (status int, errmsg string) {
	var (
		cust   *stripe.Customer
		params *stripe.CustomerParams
		err    error
	)
	params = &stripe.CustomerParams{
		Description: &guest.Name,
		Email:       &guest.Email,
	}
	if guest.Address != "" {
		params.Shipping = &stripe.CustomerShippingDetailsParams{
			Name: &guest.Name,
			Address: &stripe.AddressParams{
				Line1:      &guest.Address,
				City:       &guest.City,
				State:      &guest.State,
				PostalCode: &guest.Zip,
			},
		}
	}
	if cardSource != "" {
		params.SetSource(cardSource)
	}
	params.AddExpand("default_source")
	if cust, err = customer.Update(guest.StripeCustomer, params); err != nil {
		if serr, ok := err.(*stripe.Error); ok {
			if serr.Type == stripe.ErrorTypeCard {
				return 400, serr.Msg
			}
		}
		log.Printf("stripe update customer: %s", err)
		return 500, ""
	}
	if cust.DefaultSource != nil && cust.DefaultSource.SourceObject != nil && cust.DefaultSource.SourceObject.TypeData != nil {
		guest.StripeSource = cust.DefaultSource.ID
		guest.StripeDescription = fmt.Sprintf("%s %s",
			cust.DefaultSource.SourceObject.TypeData["brand"], cust.DefaultSource.SourceObject.TypeData["last4"])
	}
	return 200, ""
}

// GetScholaOrderNumber returns a new order number from the Schola order number
// sequence, guaranteed unique.  It returns zero if no order number is
// available.
func GetScholaOrderNumber() (onum int) {
	var req *http.Request
	var resp *http.Response
	var err error

	if req, err = http.NewRequest(http.MethodPost, config.ScholaOrderNumberURL, nil); err != nil {
		panic(err)
	}
	req.Header.Set("Auth", private.CrossSiteKey)
	if resp, err = http.DefaultClient.Do(req); err != nil {
		log.Printf("schola order number: %s", err)
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("schola order number: %s", resp.Status)
		return 0
	}
	if _, err = fmt.Fscan(resp.Body, &onum); err != nil {
		log.Printf("schola order number: %s", err)
		return 0
	}
	return onum
}

// ChargeStripe issues a charge to the payer's credit card.  payType should be
// either "cardEntry" or "cardOnFile" depending on whether the card source was
// just added to the customer or was already there.  ChargeStripe returns a
// status of 200, 400 (card declined), or 500; and (if status==400) the error
// message returned by Stripe.
func ChargeStripe(
	payer *model.Guest, payType, description, sku string,
	scholaOrder, qty, total int,
) (status int, errmsg string) {
	var (
		params  *stripe.OrderParams
		pparams *stripe.OrderPayParams
		o       *stripe.Order
		err     error
	)
	// Create the order in Stripe.
	params = &stripe.OrderParams{
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		Customer: &payer.StripeCustomer,
		Email:    &payer.Email,
		Params: stripe.Params{
			Metadata: map[string]string{
				"order-number": strconv.Itoa(scholaOrder),
				"payment-type": payType,
			},
		},
		Items: []*stripe.OrderItemParams{{
			Amount:      stripe.Int64(int64(total)),
			Currency:    stripe.String(string(stripe.CurrencyUSD)),
			Description: &description,
			Parent:      &sku,
			Quantity:    stripe.Int64(int64(qty)),
			Type:        stripe.String(string(stripe.OrderItemTypeSKU)),
		}},
	}
	if o, err = order.New(params); err != nil {
		log.Printf("stripe create order %d: %s", scholaOrder, err)
		return 500, ""
	}

	// Pay the order in Stripe.
	pparams = &stripe.OrderPayParams{Customer: &payer.StripeCustomer}
	pparams.SetSource(payer.StripeSource)
	_, err = order.Pay(o.ID, pparams)
	if err != nil {
		// Cancel the order.
		if _, err2 := order.Update(o.ID, &stripe.OrderUpdateParams{
			Status: stripe.String(string(stripe.OrderStatusCanceled)),
		}); err2 != nil {
			log.Printf("stripe cancel order %d: %s", scholaOrder, err)
		}
	}
	if serr, ok := err.(*stripe.Error); ok {
		if serr.Type == stripe.ErrorTypeCard {
			return 400, serr.Msg
		}
	}
	if err != nil {
		log.Printf("stripe pay order %d: %s", scholaOrder, err)
		return 500, ""
	}
	return 200, ""
}
