package command

import (
	"context"

	"github.com/noodlensk/shopping-list/internal/grocery/domain/list"
)

type AddItem struct {
	Name string
}

type AddItemHandler struct {
	repository list.Repository
}

func NewAddItemHandler(repository list.Repository) AddItemHandler {
	return AddItemHandler{repository: repository}
}

func (h AddItemHandler) Handle(ctx context.Context, c AddItem) error {
	return h.repository.AddItem(ctx, c.Name)
}
