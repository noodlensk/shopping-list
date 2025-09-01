package command

import (
	"context"

	"github.com/noodlensk/shopping-list/internal/grocery/domain/list"
)

type CompleteItem struct {
	Name string
}

type CompleteItemHandler struct {
	repository list.Repository
}

func NewCompleteItemHandler(repository list.Repository) CompleteItemHandler {
	return CompleteItemHandler{repository: repository}
}

func (h CompleteItemHandler) Handle(ctx context.Context, c CompleteItem) error {
	return h.repository.UpdateItem(ctx, c.Name, func(_ context.Context, i *list.Item) (*list.Item, error) {
		i.Bought = !i.Bought

		return i, nil
	})
}
