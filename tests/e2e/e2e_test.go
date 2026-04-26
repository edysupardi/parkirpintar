//go:build e2e

package e2e_test

import (
	"testing"
)

// E2E test scenarios — semua wajib dari soal
// Run: go test ./tests/e2e/... -tags e2e -v

func TestE2E_01_HappyPath(t *testing.T)                   { t.Skip("TODO") }
func TestE2E_02_DoubleBookPrevention(t *testing.T)         { t.Skip("TODO") }
func TestE2E_03_UserSelectedSpotContention(t *testing.T)   { t.Skip("TODO") }
func TestE2E_04_ReservationExpiry_NoShow(t *testing.T)     { t.Skip("TODO") }
func TestE2E_05_WrongSpotPenalty(t *testing.T)             { t.Skip("TODO") }
func TestE2E_06_CancellationUnder2Min(t *testing.T)        { t.Skip("TODO") }
func TestE2E_07_CancellationOver2Min(t *testing.T)         { t.Skip("TODO") }
func TestE2E_08_ExtendedStay_NoOverstayPenalty(t *testing.T) { t.Skip("TODO") }
func TestE2E_09_OvernightFee(t *testing.T)                 { t.Skip("TODO") }
func TestE2E_10_PaymentQRIS_Success(t *testing.T)          { t.Skip("TODO") }
func TestE2E_11_PaymentQRIS_Failure(t *testing.T)          { t.Skip("TODO") }
func TestE2E_12_PaymentVirtualAccount(t *testing.T)        { t.Skip("TODO") }
func TestE2E_13_DuplicateWebhook_Idempotent(t *testing.T)  { t.Skip("TODO") }
