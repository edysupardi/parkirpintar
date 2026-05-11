package gateway

import (
	"context"
	"crypto/sha512"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/services/payment/internal/domain"
	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
)

type MidtransGateway struct {
	client    coreapi.Client
	serverKey string
}

func NewMidtrans(serverKey string, isProduction bool) *MidtransGateway {
	env := midtrans.Sandbox
	if isProduction {
		env = midtrans.Production
	}
	c := coreapi.Client{}
	c.New(serverKey, env)
	return &MidtransGateway{client: c, serverKey: serverKey}
}

func (g *MidtransGateway) CreateQRIS(ctx context.Context, orderID string, amount int64, customer domain.CustomerInfo) (qrString, paymentURL string, expiredAt time.Time, err error) {
	req := &coreapi.ChargeReq{
		PaymentType: coreapi.PaymentTypeGopay,
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  orderID,
			GrossAmt: amount,
		},
		CustomerDetails: &midtrans.CustomerDetails{
			FName: customer.Name,
			Email: customer.Email,
			Phone: customer.Phone,
		},
	}

	resp, midErr := g.client.ChargeTransaction(req)
	if midErr != nil && midErr.Message != "" {
		return "", "", time.Time{}, fmt.Errorf("midtrans charge QRIS: %s", midErr.Message)
	}
	if resp == nil {
		return "", "", time.Time{}, fmt.Errorf("midtrans charge QRIS: empty response")
	}

	expiredAt = time.Now().Add(15 * time.Minute)

	// GoPay returns QR in actions
	for _, action := range resp.Actions {
		if action.Name == "generate-qr-code" {
			paymentURL = action.URL
		}
		if action.Name == "deeplink-redirect" && qrString == "" {
			qrString = action.URL
		}
	}
	if resp.QRString != "" {
		qrString = resp.QRString
	}
	if paymentURL == "" && len(resp.Actions) > 0 {
		paymentURL = resp.Actions[0].URL
	}

	return qrString, paymentURL, expiredAt, nil
}

func (g *MidtransGateway) CreateVA(ctx context.Context, orderID, bank string, amount int64, customer domain.CustomerInfo) (vaNumber string, expiredAt time.Time, err error) {
	bankCode := midtrans.BankBca
	switch bank {
	case "BNI":
		bankCode = midtrans.BankBni
	case "BRI":
		bankCode = midtrans.BankBri
	case "MANDIRI":
		bankCode = midtrans.BankMandiri
	}

	req := &coreapi.ChargeReq{
		PaymentType: coreapi.PaymentTypeBankTransfer,
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  orderID,
			GrossAmt: amount,
		},
		CustomerDetails: &midtrans.CustomerDetails{
			FName: customer.Name,
			Email: customer.Email,
			Phone: customer.Phone,
		},
		BankTransfer: &coreapi.BankTransferDetails{
			Bank: bankCode,
		},
	}

	resp, err := g.client.ChargeTransaction(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("midtrans charge VA: %w", err)
	}

	vaNumber = ""
	if len(resp.VaNumbers) > 0 {
		vaNumber = resp.VaNumbers[0].VANumber
	}
	expiredAt = time.Now().Add(24 * time.Hour)
	return vaNumber, expiredAt, nil
}

// VerifyWebhookSignature returns the expected HMAC-SHA512 signature.
// Caller should compare this with the signature from the webhook header.
func (g *MidtransGateway) VerifyWebhookSignature(orderID, statusCode, grossAmount, serverKey string) string {
	raw := orderID + statusCode + grossAmount + serverKey
	h := sha512.New()
	h.Write([]byte(raw))
	return fmt.Sprintf("%x", h.Sum(nil))
}
