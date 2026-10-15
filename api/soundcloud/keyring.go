package soundcloud

import (
	"errors"

	"github.com/zalando/go-keyring"
)

type KeyringStore struct{ Account string }

func (s KeyringStore) Load() (string, error) {
	return keyring.Get("sc_player.soundcloud", s.Account)
}

func (s KeyringStore) Save(token string) error {
	return keyring.Set("sc_player.soundcloud", s.Account, token)
}

func (s KeyringStore) Delete() error {
	err := keyring.Delete("sc_player.soundcloud", s.Account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
