package usage

import (
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/ledgerclose/internal/identity"
	"example.com/ledgerclose/internal/money"
)

func limits() Limits {
	return Limits{
		PageSize:         50,
		MaxBatch:         10,
		MaxBody:          4096,
		MaxEventQuantity: 1_000,
		MaxUnitAmount:    money.Amount(100_000),
		MaxLines:         100,
		MaxResponse:      4096,
	}
}

const twoEvents = `{"count":2,"events":[
{"id":"evt-0001-abcd","account":"team-api","metric":"api_call","quantity":10,"unit_amount":100,"currency":"USD","occurred_at":"2024-05-10T12:00:00Z"},
{"id":"evt-0002-abcd","account":"team-api","metric":"seat","quantity":1,"unit_amount":2500,"currency":"USD","occurred_at":"2024-05-11T12:00:00Z"}]}`

func TestDecodeObjectReadsACountedBatch(t *testing.T) {
	batch, err := DecodeObject(strings.NewReader(twoEvents), limits())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if batch.Len() != 2 {
		t.Fatalf("decoded %d events", batch.Len())
	}
	if metrics := batch.Metrics(); len(metrics) != 2 || metrics[0] != "api_call" {
		t.Fatalf("metrics = %v", metrics)
	}
}

func TestDecodeObjectRefusesACountTheBodyDoesNotBack(t *testing.T) {
	overstated := strings.Replace(twoEvents, `"count":2`, `"count":3`, 1)
	if _, err := DecodeObject(strings.NewReader(overstated), limits()); !errors.Is(err, ErrMalformed) {
		t.Fatalf("an overstated count was accepted: %v", err)
	}
	tooMany := strings.Replace(twoEvents, `"count":2`, `"count":900`, 1)
	if _, err := DecodeObject(strings.NewReader(tooMany), limits()); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a count above the batch cap was accepted: %v", err)
	}
}

func TestDecodeObjectRefusesAFieldItDoesNotKnow(t *testing.T) {
	unknown := strings.Replace(twoEvents, `"quantity":10`, `"quantity":10,"surcharge":5`, 1)
	if _, err := DecodeObject(strings.NewReader(unknown), limits()); !errors.Is(err, ErrMalformed) {
		t.Fatalf("an unknown event field was accepted: %v", err)
	}
}

func TestDecodersRefuseABodyAboveTheBound(t *testing.T) {
	small := limits()
	small.MaxBody = 64
	if _, err := DecodeObject(strings.NewReader(twoEvents), small); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("an oversized object body was accepted: %v", err)
	}
	if _, err := DecodeStream(strings.NewReader(twoEvents), small); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("an oversized stream body was accepted: %v", err)
	}
}

func TestDecodeStreamReadsTheHeaderThenTheLines(t *testing.T) {
	stream := `{"count":1}` + "\n" +
		`{"id":"evt-0003-abcd","account":"team-api","metric":"message","quantity":5,"unit_amount":20,"currency":"USD","occurred_at":"2024-05-10T12:00:00Z"}` + "\n"
	batch, err := DecodeStream(strings.NewReader(stream), limits())
	if err != nil {
		t.Fatalf("decode stream: %v", err)
	}
	if batch.Len() != 1 || batch.Events[0].ID != "evt-0003-abcd" {
		t.Fatalf("decoded %+v", batch.Events)
	}
	overstated := strings.Replace(stream, `"count":1`, `"count":2`, 1)
	if _, err := DecodeStream(strings.NewReader(overstated), limits()); !errors.Is(err, ErrMalformed) {
		t.Fatalf("a stream that claims two lines and carries one was accepted: %v", err)
	}
}

func TestBatchValidationAppliesTheCapsAndTheWindow(t *testing.T) {
	now := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
	good := Event{
		ID: "evt-0004-abcd", Account: "team-api", Metric: "api_call", Quantity: 2,
		UnitAmount: 50, Currency: "USD", OccurredAt: time.Date(2024, 5, 2, 12, 0, 0, 0, time.UTC),
	}
	if err := (Batch{Events: []Event{good}}).Validate(limits(), now); err != nil {
		t.Fatalf("a valid batch was refused: %v", err)
	}
	duplicate := Batch{Events: []Event{good, good}}
	if err := duplicate.Validate(limits(), now); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("a repeated id was accepted: %v", err)
	}
	future := good
	future.ID = "evt-0005-abcd"
	future.OccurredAt = now.Add(time.Hour)
	if err := (Batch{Events: []Event{future}}).Validate(limits(), now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an event from the future was accepted: %v", err)
	}
	big := good
	big.ID = "evt-0006-abcd"
	big.Quantity = 10_000
	if err := (Batch{Events: []Event{big}}).Validate(limits(), now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a quantity above the cap was accepted: %v", err)
	}
}

func TestCheckAccountsTiesAnEventToTheTenant(t *testing.T) {
	directory := identity.NewDirectory()
	account := identity.Account{
		ID: "team-api", Tenant: "acme", Subject: "amara", DisplayName: "Team API",
		Email: "billing@example.com", Currency: "USD", RateTable: "standard",
	}
	if _, err := directory.Add(account); err != nil {
		t.Fatalf("register: %v", err)
	}
	event := Event{
		ID: "evt-0007-abcd", Account: "team-api", Metric: "api_call", Quantity: 1,
		UnitAmount: 50, Currency: "USD", OccurredAt: time.Date(2024, 5, 2, 12, 0, 0, 0, time.UTC),
	}
	if err := (Batch{Events: []Event{event}}).CheckAccounts(directory, "acme"); err != nil {
		t.Fatalf("a valid event was refused: %v", err)
	}
	if err := (Batch{Events: []Event{event}}).CheckAccounts(directory, "globex"); err == nil {
		t.Fatal("an event of another tenant was accepted")
	}
	wrongCurrency := event
	wrongCurrency.Currency = "EUR"
	if err := (Batch{Events: []Event{wrongCurrency}}).CheckAccounts(directory, "acme"); err == nil {
		t.Fatal("an event in another currency was accepted")
	}
}
