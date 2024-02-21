package instrumentation

import (
	"context"
	"log/slog"
)

const (
	// List of instrumentation using text map carrier. The text map carrier instrumentation is different
	// from http.Header or gRPC metadata because the text map carrier can carry the instrumentation using
	// map[string]string attributes.
	//
	// While http.Header is also a map[string]string, but we don't want to make things complicated by combining
	// two different things into one.
	requestIDInst         = "inst-request-id"
	apiNameInst           = "inst-bff-api-name"
	apiOwnerInst          = "inst-bff-api-owner"
	forwardedForInst      = "inst-forwarded-for"
	debugIDInst           = "inst-debug-id"
	preferredLanguageInst = "inst-preferred-language"
)

// baggageKey is the struct key that is used to store Baggage information inside context.Context.
var baggageKey struct{}

// Baggage is the instrumentation bagge that will be included in context.Context to ensure
// all informations are propagated.
//
// Please NOTE that not all headers/metadata are included in the instrumentation, because we
// only ensure we have something that need to be propageted consistently across multiple services.
type Baggage struct {
	RequestID         string // RequestID is the unique id for every request.
	APIName           string // APIname is the BFF api name for the request.
	APIOwner          string // APIOwner is the BFF api owner for the request.
	DebugID           string // DebugID is a special id to identify a debug request.
	PreferredLanguage string // PreferredLanguage is a parameter for the langauge usage.
	// valid is a flag to check whether the baggage is a valid baggage that created from the
	// http header, gRPC metadata or something else that supported in the instrumentation package.
	valid bool
}

// ToSlogAttributes returns the slog attributes from the baggage.
func (b Baggage) ToSlogAttributes() []slog.Attr {
	// If the bagge is not valid, we will return an empty slice so the caller can re-use
	// the slice if needed.
	if !b.valid {
		return []slog.Attr{}
	}
	return []slog.Attr{
		{
			Key:   "request.id",
			Value: slog.StringValue(b.RequestID),
		},
		{
			Key:   "api.name",
			Value: slog.StringValue(b.APIName),
		},
		{
			Key:   "api.owner",
			Value: slog.StringValue(b.APIOwner),
		},
		{
			Key:   "debug.id",
			Value: slog.StringValue(b.DebugID),
		},
	}
}

// ToTextMapCarrier transform the baggage into map[string]string as the carrier.
//
// This function can be used if we were about to propagate informations to a message-bus
// platform because these platforms usually supports attributes in the form of map[string]string.
func (b Baggage) ToTextMapCarrier() map[string]string {
	// Please adjust the length of the text map carrier based on the baggage KV.
	carrier := make(map[string]string, 6)
	carrier[requestIDInst] = b.RequestID
	carrier[apiNameInst] = b.APIName
	carrier[apiOwnerInst] = b.APIOwner
	carrier[debugIDInst] = b.DebugID
	carrier[preferredLanguageInst] = b.PreferredLanguage
	return carrier
}

// BaggageFromTextMapCarrier creates a new baggage from text map carrier. This kind of format is used to easily inject them to
// another protocol like HTTP(header) and gRPC(metadata). This is why several observability provider also use this kind of format.
func BaggageFromTextMapCarrier(carrier map[string]string) Baggage {
	baggage := Baggage{
		RequestID:         carrier[requestIDInst],
		APIName:           carrier[apiNameInst],
		APIOwner:          carrier[apiOwnerInst],
		DebugID:           carrier[debugIDInst],
		PreferredLanguage: carrier[preferredLanguageInst],
		valid:             true,
	}
	return baggage
}

// BaggageFromContext returns the insturmented bagage from a context.Context.
func BaggageFromContext(ctx context.Context) Baggage {
	baggage, ok := ctx.Value(baggageKey).(Baggage)
	if !ok {
		return Baggage{}
	}
	return baggage
}

// ContextWithBaggage set the context key using the passed baggage value.
func ContextWithBaggage(ctx context.Context, bg Baggage) context.Context {
	return context.WithValue(ctx, baggageKey, bg)
}
