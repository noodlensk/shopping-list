package query

import (
	"context"

	"github.com/noodlensk/shopping-list/internal/grocery/domain/list"
)

type GetItems struct{}

type Items []list.Item

type GetItemsHandler struct {
	repository list.Repository
}

func NewGetItemsHandler(repository list.Repository) GetItemsHandler {
	return GetItemsHandler{repository: repository}
}

func (h GetItemsHandler) Handle(ctx context.Context, c GetItems) (Items, error) {
	return h.repository.ListItems(ctx)
}
