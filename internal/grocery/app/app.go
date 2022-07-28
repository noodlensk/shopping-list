package app

import (
	"github.com/noodlensk/shopping-list/internal/grocery/app/command"
	"github.com/noodlensk/shopping-list/internal/grocery/app/query"
)

type Commands struct {
	AddItem              command.AddItemHandler
	RemoveCompletedItems command.RemoveCompetedItemsHandler
	CompleteItem         command.CompleteItemHandler
}

type Queries struct {
	GetItems query.GetItemsHandler
}

type Application struct {
	Commands Commands
	Queries  Queries
}
