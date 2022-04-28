// Package lambdinte handles Discord interactions received as AWS Lambda events.
// It is largely modeled after net/http; register interaction handlers with DefaultMux via top-level Register functions,
// and then call Start to listen for events.
// Clients wanting more control can set the Handler fields of DefaultMux or create a Function with a custom base Handler.
package lambdinte

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/bwmarrin/discordgo"
)

// Function handles Discord interactions received as AWS Lambda events.
type Function struct {
	// PublicKey should be set to your Discord application's public key before calling Start.
	PublicKey ed25519.PublicKey
	// Handler handles incoming interaction events; if it is nil, DefaultMux will be used.
	Handler Handler
}

type incomingEvent struct {
	Body            string            `json:"body"`
	Headers         map[string]string `json:"headers"`
	IsBase64Encoded bool              `json:"isBase64Encoded"`
}

type outgoingResult struct {
	StatusCode int                            `json:"statusCode"`
	Response   *discordgo.InteractionResponse `json:"body,omitempty"`
}

// Invoke reads the event data, verifies the signature, and calls Handler.Handle.
func (f *Function) Invoke(ctx context.Context, eventData []byte) ([]byte, error) {
	var evt incomingEvent
	err := json.Unmarshal(eventData, &evt)
	if err != nil {
		return nil, err
	}

	result, err := f.invoke(ctx, evt)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (f *Function) invoke(ctx context.Context, evt incomingEvent) (res outgoingResult, err error) {
	signature, ok := evt.Headers["X-Signature-Ed25519"]
	if !ok {
		res.StatusCode = 401
		return
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		err = nil
		res.StatusCode = 401
		return
	}

	timestamp, ok := evt.Headers["X-Signature-Timestamp"]
	if !ok {
		res.StatusCode = 401
		return
	}

	var body []byte
	if evt.IsBase64Encoded {
		body, err = base64.StdEncoding.DecodeString(evt.Body)
		if err != nil {
			return
		}
	} else {
		body = []byte(evt.Body)
	}

	var signed bytes.Buffer
	signed.Grow(len(timestamp) + len(body))
	_, err = io.Copy(&signed, strings.NewReader(timestamp))
	if err != nil {
		return
	}
	_, err = io.Copy(&signed, bytes.NewReader(body))
	if err != nil {
		return
	}
	if !ed25519.Verify(f.PublicKey, signed.Bytes(), sig) {
		res.StatusCode = 401
		return
	}

	var interaction discordgo.Interaction
	err = json.Unmarshal(body, &interaction)
	if err != nil {
		// this defo came from Discord, so it's our fault if we can't read it
		return
	}

	res.StatusCode = 200
	res.Response = new(discordgo.InteractionResponse)
	*res.Response, err = f.Handler.Handle(ctx, interaction)
	return
}

// Start listens for incoming AWS Lambda events.
// If PublicKey is nil, Start panics.
func (f *Function) Start() {
	if f.PublicKey == nil {
		panic("Start called but PublicKey is still nil, please set up your public key")
	}

	if f.Handler == nil {
		f.Handler = DefaultMux
	}

	lambda.StartHandler(f)
}

// Start listens for incoming AWS Lambda events using the given public key and DefaultMux.
func Start(publicKey ed25519.PublicKey) {
	(&Function{PublicKey: publicKey, Handler: DefaultMux}).Start()
}
