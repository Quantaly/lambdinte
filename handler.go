package lambdinte

import (
	"context"

	"github.com/bwmarrin/discordgo"
)

// Handler handles and responds to Discord interactions
type Handler interface {
	Handle(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)
}

// HandlerFunc is a Handler that calls itself
type HandlerFunc func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)

// Handle calls the underlying function
func (f HandlerFunc) Handle(c context.Context, i discordgo.Interaction) (discordgo.InteractionResponse, error) {
	return f(c, i)
}

type handlerMux struct {
	// http.ServeMux has some schmancy stuff with, like, mutexes and stuff.
	// maybe we need some of that?
	// eh probably not

	handlers map[string]Handler
}

// Register registers the handler for the given key.
// If a handler already exists for key, Handle panics.
func (h *handlerMux) Register(key string, handler Handler) {
	if handler == nil {
		panic("lambdinte: nil handler")
	}
	if _, exists := h.handlers[key]; exists {
		panic("lambdinte: multiple registrations for " + key)
	}

	h.handlers[key] = handler
}

// RegisterFunc registers the handler function for the given key.
func (h *handlerMux) RegisterFunc(name string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	if handler == nil {
		panic("lambdinte: nil handler")
	}

	h.Register(name, HandlerFunc(handler))
}

// ApplicationCommandMux stores and selects handlers for application command interactions based on their name.
// It is appropriate for both APPLICATION_COMMAND interactions (type 2) and APPLICATION_COMMAND_AUTOCOMPLETE interactions (type 4).
type ApplicationCommandMux struct {
	handlerMux
}

// Handle handles the interaction. If the interaction is of the wrong type, or the command name has not been registered, it panics.
func (m *ApplicationCommandMux) Handle(ctx context.Context, evt discordgo.Interaction) (discordgo.InteractionResponse, error) {
	if evt.Type != discordgo.InteractionApplicationCommand && evt.Type != discordgo.InteractionApplicationCommandAutocomplete {
		panic("lambdinte: ApplicationCommandMux asked to handle interaction of wrong type " + evt.Type.String())
	}

	name := evt.ApplicationCommandData().Name
	if handler, ok := m.handlers[name]; ok {
		return handler.Handle(ctx, evt)
	}

	panic("lambdinte: ApplicationCommandMux asked to handle unknown command " + name)
}

// MessageComponentMux stores and selects handlers for message component interactions based on their custom_id.
// If you use custom_id for any purpose other than identifying the component, such as persisting state, it is probably not appropriate.
type MessageComponentMux struct {
	handlerMux
}

// Handle handles the interaction. If the interaction is of the wrong type, or the component ID has not been registered, it panics.
func (m *MessageComponentMux) Handle(ctx context.Context, evt discordgo.Interaction) (discordgo.InteractionResponse, error) {
	if evt.Type != discordgo.InteractionMessageComponent {
		panic("lambdinte: MessageComponentMux asked to handle interaction of wrong type " + evt.Type.String())
	}

	customID := evt.MessageComponentData().CustomID
	if handler, ok := m.handlers[customID]; ok {
		return handler.Handle(ctx, evt)
	}

	panic("lambdinte: MessageComponentMux asked to handle unknown ID " + customID)
}

// ModalSubmitMux stores and selects handlers for modal submit interactions based on their custom_id.
// If you use custom_id for any purpose other than identifying the modal, such as persisting state, it is probably not appropriate.
type ModalSubmitMux struct {
	handlerMux
}

// Handle handles the interaction. If the interaction is of the wrong type, or the modal ID has not been registered, it panics.
func (m *ModalSubmitMux) Handle(ctx context.Context, evt discordgo.Interaction) (discordgo.InteractionResponse, error) {
	if evt.Type != discordgo.InteractionModalSubmit {
		panic("lambdinte: ModalSubmitMux asked to handle interaction of wrong type " + evt.Type.String())
	}

	customID := evt.ModalSubmitData().CustomID
	if handler, ok := m.handlers[customID]; ok {
		return handler.Handle(ctx, evt)
	}

	panic("lambdinte: ModalSubmitMux asked to handle unknown ID " + customID)
}

// Mux routes interactions to Handlers based on their types.
type Mux struct {
	// PingHandler is called for PING interactions (type 1).
	// If it is nil, DefaultPingHandler will be used.
	PingHandler Handler
	// ApplicationCommandHandler is called for APPLICATION_COMMAND interactions (type 2).
	// You probably want to use RegisterCommand and/or RegisterCommandFunc to set it up.
	ApplicationCommandHandler Handler
	// MessageComponentHandler is called for MESSAGE_COMPONENT interactions (type 3).
	// You may want to use RegisterComponent and/or RegisterComponentFunc to set it up.
	MessageComponentHandler Handler
	// ApplicationCommandAutocompleteHandler is called for APPLICATION_COMMAND_AUTOCOMPLETE interactions (type 4).
	// You probably want to use RegisterCommandAutocomplete and/or RegisterCommandAutocompleteFunc to set it up.
	ApplicationCommandAutocompleteHandler Handler
	// ModalSubmitHandler is called for MODAL_SUBMIT interactions (type 5).
	// You may want to use RegisterModal and/or RegisterModalFunc to set it up.
	ModalSubmitHandler Handler
}

// Handle forwards to the appropriate Handler.
// If an interaction is received of a type that Mux does not have a handler for (other than PING), Handle panics.
func (m *Mux) Handle(ctx context.Context, evt discordgo.Interaction) (discordgo.InteractionResponse, error) {
	switch evt.Type {
	case discordgo.InteractionPing:
		if m.PingHandler == nil {
			return DefaultPingHandlerFunc(ctx, evt)
		}
		return m.PingHandler.Handle(ctx, evt)
	case discordgo.InteractionApplicationCommand:
		if m.ApplicationCommandHandler == nil {
			panic("lambdinte: Mux asked to handle application command interaction but ApplicationCommandHandler is nil")
		}
		return m.ApplicationCommandHandler.Handle(ctx, evt)
	case discordgo.InteractionMessageComponent:
		if m.MessageComponentHandler == nil {
			panic("lambdinte: Mux asked to handle message component interaction but MessageComponentHandler is nil")
		}
		return m.MessageComponentHandler.Handle(ctx, evt)
	case discordgo.InteractionApplicationCommandAutocomplete:
		if m.ApplicationCommandAutocompleteHandler == nil {
			panic("lambdinte: Mux asked to handle application command autocomplete interaction but ApplicationCommandAutocompleteHandler is nil")
		}
		return m.ApplicationCommandAutocompleteHandler.Handle(ctx, evt)
	case discordgo.InteractionModalSubmit:
		if m.ModalSubmitHandler == nil {
			panic("lambdinte: Mux asked to handle modal submit interaction but ModalSubmitHandler is nil")
		}
		return m.ModalSubmitHandler.Handle(ctx, evt)
	}

	panic("lambdinte: Mux asked to handle interaction of unknown type " + evt.Type.String())
}

// RegisterCommand registers the handler for application command interactions with the given name.
// If ApplicationCommandHandler is nil, it is set up as an ApplicationCommandMux.
// If ApplicationCommandHandler is not nil or an ApplicationCommandMux, RegisterCommand panics.
func (m *Mux) RegisterCommand(name string, handler Handler) {
	if m.ApplicationCommandHandler == nil {
		m.ApplicationCommandHandler = new(ApplicationCommandMux)
	}

	if mux, ok := m.ApplicationCommandHandler.(*ApplicationCommandMux); ok {
		mux.Register(name, handler)
	} else {
		panic("lambdinte: RegisterCommand called but ApplicationCommandHandler is not nil or an ApplicationCommandMux")
	}
}

// RegisterCommandFunc registers the handler function for application command interactions with the given name.
// If ApplicationCommandHandler is nil, it is set up as an ApplicationCommandMux.
// If ApplicationCommandHandler is not nil or an ApplicationCommandMux, RegisterCommandFunc panics.
func (m *Mux) RegisterCommandFunc(name string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	m.RegisterCommand(name, HandlerFunc(handler))
}

// RegisterComponent registers the handler for message component interactions with the given ID.
// If MessageComponentHandler is nil, it is set up as a MessageComponentMux.
// If MessageComponentHandler is not nil or a MessageComponentMux, RegisterComponent panics.
func (m *Mux) RegisterComponent(customID string, handler Handler) {
	if m.MessageComponentHandler == nil {
		m.MessageComponentHandler = new(MessageComponentMux)
	}

	if mux, ok := m.MessageComponentHandler.(*MessageComponentMux); ok {
		mux.Register(customID, handler)
	} else {
		panic("lambdinte: RegisterComponent called but MessageComponentHandler is not nil or a MessageComponentMux")
	}
}

// RegisterComponentFunc registers the handler function for message component interactions with the given ID.
// If MessageComponentHandler is nil, it is set up as a MessageComponentMux.
// If MessageComponentHandler is not nil or a MessageComponentMux, RegisterComponentFunc panics.
func (m *Mux) RegisterComponentFunc(customID string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	m.RegisterComponent(customID, HandlerFunc(handler))
}

// RegisterCommandAutocomplete registers the handler for application command autocomplete interactions with the given name.
// If ApplicationCommandAutocompleteHandler is nil, it is set up as an ApplicationCommandMux.
// If ApplicationCommandAutocompleteHandler is not nil or an ApplicationCommandMux, RegisterCommandAutocomplete panics.
func (m *Mux) RegisterCommandAutocomplete(name string, handler Handler) {
	if m.ApplicationCommandAutocompleteHandler == nil {
		m.ApplicationCommandAutocompleteHandler = new(ApplicationCommandMux)
	}

	if mux, ok := m.ApplicationCommandAutocompleteHandler.(*ApplicationCommandMux); ok {
		mux.Register(name, handler)
	} else {
		panic("lambdinte: RegisterCommandAutocomplete called but ApplicationCommandAutocompleteHandler is not nil or an ApplicationCommandMux")
	}
}

// RegisterCommandAutocompleteFunc registers the handler function for application command autocomplete interactions with the given name.
// If ApplicationCommandAutocompleteHandler is nil, it is set up as an ApplicationCommandMux.
// If ApplicationCommandAutocompleteHandler is not nil or an ApplicationCommandMux, RegisterCommandAutocompleteFunc panics.
func (m *Mux) RegisterCommandAutocompleteFunc(name string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	m.RegisterCommandAutocomplete(name, HandlerFunc(handler))
}

// RegisterModal registers the handler for modal submit interactions with the given ID.
// If ModalSubmitHandler is nil, it is set up as a ModalSubmitMux.
// If ModalSubmitHandler is not nil or a ModalSubmitMux, RegisterModal panics.
func (m *Mux) RegisterModal(customID string, handler Handler) {
	if m.ModalSubmitHandler == nil {
		m.ModalSubmitHandler = new(ModalSubmitMux)
	}

	if mux, ok := m.ModalSubmitHandler.(*ModalSubmitMux); ok {
		mux.Register(customID, handler)
	} else {
		panic("lambdinte: RegisterModal called but ModalSubmitHandler is not nil or a ModalSubmitMux")
	}
}

// RegisterModalFunc registers the handler function for modal submit interactions with the given ID.
// If ModalSubmitHandler is nil, it is set up as a ModalSubmitMux.
// If ModalSubmitHandler is not nil or a ModalSubmitMux, RegisterModalFunc panics.
func (m *Mux) RegisterModalFunc(customID string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	m.RegisterModal(customID, HandlerFunc(handler))
}

// DefaultPingHandler responds to pings with pongs.
var DefaultPingHandler = HandlerFunc(DefaultPingHandlerFunc)

// DefaultPingHandlerFunc responds to pings with pongs.
func DefaultPingHandlerFunc(ctx context.Context, evt discordgo.Interaction) (discordgo.InteractionResponse, error) {
	return discordgo.InteractionResponse{Type: discordgo.InteractionResponsePong}, nil
}

// DefaultMux is the default Handler used by App.
var DefaultMux = &defaultMux

var defaultMux Mux

// RegisterCommand registers the handler for application command interactions with the given name into DefaultMux.
func RegisterCommand(name string, handler Handler) {
	DefaultMux.RegisterCommand(name, handler)
}

// RegisterCommandFunc registers the handler function for application command interactions with the given name into DefaultMux.
func RegisterCommandFunc(name string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	DefaultMux.RegisterCommand(name, HandlerFunc(handler))
}

// RegisterComponent registers the handler for message component interactions with the given ID into DefaultMux.
func RegisterComponent(customID string, handler Handler) {
	DefaultMux.RegisterComponent(customID, handler)
}

// RegisterComponentFunc registers the handler function for message component interactions with the given ID into DefaultMux.
func RegisterComponentFunc(customID string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	DefaultMux.RegisterComponent(customID, HandlerFunc(handler))
}

// RegisterCommandAutocomplete registers the handler for application command autocomplete interactions with the given name into DefaultMux.
func RegisterCommandAutocomplete(name string, handler Handler) {
	DefaultMux.RegisterCommandAutocomplete(name, handler)
}

// RegisterCommandAutocompleteFunc registers the handler function for application command autocomplete interactions with the given name into DefaultMux.
func RegisterCommandAutocompleteFunc(name string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	DefaultMux.RegisterCommandAutocomplete(name, HandlerFunc(handler))
}

// RegisterModal registers the handler for modal submit interactions with the given ID into DefaultMux.
func RegisterModal(customID string, handler Handler) {
	DefaultMux.RegisterModal(customID, handler)
}

// RegisterModalFunc registers the handler function for modal submit interactions with the given ID into DefaultMux.
func RegisterModalFunc(customID string, handler func(context.Context, discordgo.Interaction) (discordgo.InteractionResponse, error)) {
	DefaultMux.RegisterModal(customID, HandlerFunc(handler))
}
