package command

import (
	"context"
	"fmt"

	"github.com/noodlensk/shopping-list/internal/grocery/domain/list"
)

type RemoveCompetedItems struct{}

type RemoveCompetedItemsHandler struct {
	repository list.Repository
}

func NewRemoveCompetedItemsHandler(repository list.Repository) RemoveCompetedItemsHandler {
	return RemoveCompetedItemsHandler{repository: repository}
}

func (h RemoveCompetedItemsHandler) Handle(ctx context.Context, c RemoveCompetedItems) error {
	itemsList, err := h.repository.ListItems(ctx)
	if err != nil {
		return fmt.Errorf("list items: %w", err)
	}

	for _, i := range itemsList {
		if i.Bought {
			if err := h.repository.DeleteItem(ctx, i.Name); err != nil {
				return fmt.Errorf("delete item %q: %w", i.Name, err)
			}
		}
	}

	return nil
}
