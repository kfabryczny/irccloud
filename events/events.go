package events

import (
	"encoding/json"
	"fmt"
	"github.com/termoose/irccloud/requests"
	"github.com/termoose/irccloud/ui"
	_ "log"
	"time"
)

type eventHandler struct {
	Queue        chan eventData
	SessionToken string
	Window       *ui.View
}

func NewHandler(token string, w *ui.View) *eventHandler {
	handler := &eventHandler{
		Queue:        make(chan eventData, 8),
		SessionToken: token,
		Window:       w,
	}

	// Start consumer thread
	go func() {
		for currEvent := range handler.Queue {
			handler.handle(currEvent, false)
		}
	}()

	return handler
}

func (e *eventHandler) Enqueue(msg []byte) {
	current := eventData{}
	err := json.Unmarshal(msg, &current)

	if err == nil {
		// Attach raw message data
		current.Data = msg

		e.Queue <- current
	}
}

func (e *eventHandler) handleBacklog(url string) {
	backlogResponse := requests.GetBacklog(e.SessionToken, url)
	backlogData := parseBacklog(backlogResponse)

	// First we initialize all channels
	for _, event := range backlogData {
		if event.Type == "channel_init" {
			userStr := []string{}
			for _, userString := range event.Members {
				userStr = append(userStr, userString.Nick)
			}

			topic := getTopicName(event.Topic)
			e.Window.AddChannel(event.Chan, topic, event.Cid, event.Bid, userStr)
		}
	}

	// Then we fill them with the message backlog, should we send these events
	// to the event queue to have them arrive before live events?
	for _, event := range backlogData {
		e.handle(event, true)
	}

	// Go to the last visited channel if it exists
	e.Window.SetLatestChannel()
	e.Window.Redraw()
}

func (e *eventHandler) handle(curr eventData, backlogEvent bool) {
	switch curr.Type {
	case "oob_include":
		oobData := &oobInclude{}
		err := json.Unmarshal(curr.Data, &oobData)

		if err == nil {
			e.handleBacklog(oobData.Url)
		}

	case "channel_init":
		if !backlogEvent {
			userStrings := []string{}
			for _, userString := range curr.Members {
				userStrings = append(userStrings, userString.Nick)
			}
			topic := getTopicName(curr.Topic)
			e.Window.AddChannel(curr.Chan, topic, curr.Cid, curr.Bid, userStrings)
		}

	case "you_parted_channel":
		if !backlogEvent {
			e.Window.RemoveChannel(curr.Chan)
		}

	case "buffer_msg":
		if e.Window.HasChannel(curr.Chan) {
			e.Window.Activity.RegisterActivity(curr.Chan, curr.Msg, e.Window)
			e.Window.AddBufferMsg(curr.Chan, curr.From, curr.Msg, curr.Time, curr.Bid)
		}

	case "joined_channel":
		if !backlogEvent {
			e.Window.AddUser(curr.Chan, curr.Nick, curr.Bid)
		}
		e.Window.AddJoinEvent(curr.Chan, curr.Nick, curr.Hostmask, curr.Time, curr.Bid)

	case "parted_channel":
		if !backlogEvent {
			e.Window.RemoveUser(curr.Chan, curr.Nick, curr.Bid)
		}
		e.Window.AddPartEvent(curr.Chan, curr.Nick, curr.Hostmask, curr.Time, curr.Bid)

	case "nickchange":
		e.Window.ChangeUserNick(curr.Chan, curr.OldNick, curr.NewNick, curr.Time, curr.Bid)

	case "channel_topic":
		e.Window.ChangeTopic(curr.Chan, curr.Author, getTopicText(curr.Topic), curr.Time, curr.Bid)

	case "makebuffer":
		if curr.BufferType == "conversation" {
			header := fmt.Sprintf("Chatting since: %s", unixtimeToDate(curr.Created))
			e.Window.AddChannel(curr.Name, header, curr.Cid, curr.Bid, []string{})
		}

	case "buffer_me_msg":
		e.Window.AddBufferMsg(curr.Chan, curr.From, curr.Msg, curr.Time, curr.Bid)

	case "quit":
		if !backlogEvent {
			e.Window.RemoveUser(curr.Chan, curr.Nick, curr.Bid)
		}
		e.Window.AddQuitEvent(curr.Chan, curr.Nick, curr.Hostmask, curr.Msg, curr.Time, curr.Bid)
	default:
		//fmt.Printf("Event: %s\n", curr.Type)
		return
	}

	// We only redraw per event if it's not a backlog event to speed
	// up app start time
	if !backlogEvent {
		e.Window.Redraw()
	}
}

func unixtimeToDate(t int64) string {
	tm := time.Unix(t/1000000, 0)
	return tm.Format("Mon Jan 2 15:04:05 UTC 2006")
}
