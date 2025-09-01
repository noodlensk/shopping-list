package ports

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/noodlensk/shopping-list/internal/grocery/app"
	"github.com/noodlensk/shopping-list/internal/grocery/app/command"
	"github.com/noodlensk/shopping-list/internal/grocery/app/query"
	"github.com/noodlensk/shopping-list/internal/grocery/domain/list"
	"go.uber.org/zap"
)

func NewTelegram(token string, allowedUsersID []int64, application *app.Application, logger *zap.SugaredLogger) error {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("new bot api: %w", err)
	}

	logger.Infof("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		logger.Infow("Got message",
			"user_name", update.Message.From.String(),
			"user_id", update.Message.From.ID,
			"msg", update.Message.Text,
		)

		if !isUserAllowed(update.Message.From.ID, allowedUsersID) {
			logger.Warn("Not allowed user, ignoring")

			continue
		}

		if update.Message == nil { // ignore non-Message updates
			continue
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)

		ctx := context.Background()

		switch update.Message.Text {
		case "/start":
			continue
		case "/refresh":
			break
		case "/clear":
			if err := application.Commands.RemoveCompletedItems.Handle(ctx, command.RemoveCompetedItems{}); err != nil {
				return fmt.Errorf("clear compleated: %w", err)
			}
		default:
			itemName := update.Message.Text

			itemList, err := application.Queries.GetItems.Handle(ctx, query.GetItems{})
			if err != nil {
				return fmt.Errorf("get items: %w", err)
			}

			found := false

			for _, i := range itemList {
				if i.Name == itemName {
					found = true

					if err := application.Commands.CompleteItem.Handle(ctx, command.CompleteItem{Name: i.Name}); err != nil {
						return fmt.Errorf("mark item as bought: %w", err)
					}

					break
				}
			}

			if !found {
				if err := application.Commands.AddItem.Handle(ctx, command.AddItem{Name: itemName}); err != nil {
					return fmt.Errorf("mark item as bought: %w", err)
				}
			}
		}

		itemList, err := application.Queries.GetItems.Handle(ctx, query.GetItems{})
		if err != nil {
			return fmt.Errorf("get items: %w", err)
		}

		var kb interface{}

		txt := "You've done with groceries!"

		if len(itemList) > 0 {
			txt = text(itemList)
		}

		kb = keyboard(itemList)

		msg.ReplyMarkup = kb
		msg.Text = txt

		if _, err := bot.Send(msg); err != nil {
			return fmt.Errorf("send message: %w", err)
		}
	}

	return nil
}

func text(list []list.Item) string {
	var res []string

	for _, item := range list {
		txt := item.Name

		if item.Bought {
			txt += " ✅"
		}

		res = append(res, txt)
	}

	return strings.Join(res, "\n")
}

func keyboard(list []list.Item) interface{} {
	numericKeyboard := tgbotapi.NewReplyKeyboard()

	for _, item := range list {
		txt := item.Name

		if item.Bought {
			continue
		}

		numericKeyboard.Keyboard = append(numericKeyboard.Keyboard, tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(txt)))
	}

	if len(numericKeyboard.Keyboard) == 0 {
		return tgbotapi.NewRemoveKeyboard(true)
	}

	return numericKeyboard
}

type List struct {
	items []*ListItem
}

type ListItem struct {
	Name   string
	Bought bool
}

func (l *List) Toggle(item string) {
	itemText := item
	toDelete := false
	found := false

	if strings.HasSuffix(item, " ✅") {
		itemText = strings.TrimSuffix(item, " ✅")
		toDelete = true
	}

	for i, val := range l.items {
		if val.Name == itemText {
			if toDelete {
				copy(l.items[i:], l.items[i+1:])
				l.items[len(l.items)-1] = nil
				l.items = l.items[:len(l.items)-1]

				return
			}

			val.Bought = !val.Bought
			found = true

			break
		}
	}

	if !found {
		l.items = append(l.items, &ListItem{Name: itemText})
	}

	sort.Slice(l.items, func(i, j int) bool {
		res := strings.Compare(l.items[i].Name, l.items[j].Name)
		iBought := l.items[i].Bought
		jBought := l.items[j].Bought

		if iBought && jBought || !iBought && !jBought {
			return res < 0
		}

		if iBought {
			return false
		}

		return true
	})
}

func isUserAllowed(id int64, allowedUsersID []int64) bool {
	for _, i := range allowedUsersID {
		if i == id {
			return true
		}
	}

	return false
}
