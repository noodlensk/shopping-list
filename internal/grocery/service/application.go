package service

import (
	"github.com/boltdb/bolt"
	"github.com/noodlensk/shopping-list/internal/grocery/adapters"
	"github.com/noodlensk/shopping-list/internal/grocery/app"
	"github.com/noodlensk/shopping-list/internal/grocery/app/command"
	"github.com/noodlensk/shopping-list/internal/grocery/app/query"
)

func NewApplication(db *bolt.DB) (*app.Application, error) {
	repo, err := adapters.NewListBoltDBRepository(db)
	if err != nil {
		return nil, err
	}

	return &app.Application{
		Commands: app.Commands{
			AddItem:              command.NewAddItemHandler(repo),
			CompleteItem:         command.NewCompleteItemHandler(repo),
			RemoveCompletedItems: command.NewRemoveCompetedItemsHandler(repo),
		},
		Queries: app.Queries{
			GetItems: query.NewGetItemsHandler(repo),
		},
	}, nil
}
