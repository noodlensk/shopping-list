package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/noodlensk/shopping-list/internal/grocery/domain/list"
)

type ListBoltDBRepository struct{ db *bolt.DB }

func NewListBoltDBRepository(db *bolt.DB) (*ListBoltDBRepository, error) {
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("grocery"))

		return err
	}); err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}

	return &ListBoltDBRepository{db: db}, nil
}

func (r ListBoltDBRepository) AddItem(_ context.Context, name string) error {
	model := list.Item{Name: name}

	data, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("marshal entity: %w", err)
	}

	return r.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("grocery")).Put([]byte(model.Name), data)
	})
}

func (r ListBoltDBRepository) UpdateItem(ctx context.Context, name string, updateFn func(ctx context.Context, i *list.Item) (*list.Item, error)) error {
	err := r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("grocery"))

		itemRaw := b.Get([]byte(name))
		if itemRaw != nil {
			a := list.Item{}
			if err := json.Unmarshal(itemRaw, &a); err != nil {
				return fmt.Errorf("unmarshal entity: %w", err)
			}

			updated, err := updateFn(ctx, &a)
			if err != nil {
				return fmt.Errorf("update func: %w", err)
			}

			updatedRaw, err := json.Marshal(updated)
			if err != nil {
				return fmt.Errorf("marshal entity: %w", err)
			}

			return b.Put([]byte(name), updatedRaw)
		}

		return list.NotFoundError{Item: name}
	})

	return err
}

func (r ListBoltDBRepository) DeleteItem(_ context.Context, name string) error {
	err := r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("grocery"))
		data := b.Get([]byte(name))

		if data != nil {
			return b.Delete([]byte(name))
		}

		return list.NotFoundError{Item: name}
	})

	return err
}

func (r ListBoltDBRepository) ListItems(_ context.Context) ([]list.Item, error) {
	var items []list.Item

	err := r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("grocery"))

		err := b.ForEach(func(_, v []byte) error {
			a := list.Item{}
			if err := json.Unmarshal(v, &a); err != nil {
				return fmt.Errorf("unmarshal entity: %w", err)
			}

			items = append(items, a)

			return nil
		})
		if err != nil {
			return err
		}

		// sort by alphabet, bought items in the end
		sort.Slice(items, func(i, j int) bool {
			res := strings.Compare(items[i].Name, items[j].Name)
			iBought := items[i].Bought
			jBought := items[j].Bought

			if iBought && jBought || !iBought && !jBought {
				return res < 0
			}

			if iBought {
				return false
			}

			return true
		})

		return nil
	})

	return items, err
}
