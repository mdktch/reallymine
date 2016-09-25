// 24 september 2016
package main

import (
	"fmt"
	"io"
	"crypto/aes"

	"github.com/andlabs/reallymine/disk"
	"github.com/andlabs/reallymine/bridge"
	"github.com/andlabs/reallymine/kek"
)

type Decrypter struct {
	Disk		*disk.Disk
	Out		io.Writer

	EncryptedKeySector		[]byte
	KeySectorPos			int64
	Bridge				bridge.Bridge

	KEK			[]byte
	KeySector		bridge.KeySector
	DEK			[]byte
}

func (d *Decrypter) FindKeySector() error {
	iter, err := d.Disk.ReverseIter(d.Disk.Size())
	if err != nil {
		return err
	}
	for iter.Next() {
		d.EncryptedKeySector = iter.Sectors()
		d.KeySectorPos = iter.Pos()
		d.Bridge = bridge.IdentifyKeySector(d.EncryptedKeySector)
		if d.Bridge != nil {
			break
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if d.Bridge == nil {
		return fmt.Errorf("key sector not found")
	}
	return nil
}

func (d *Decrypter) decryptKeySector() (err error) {
	d.KeySector, err = d.Bridge.DecryptKeySector(d.EncryptedKeySector, d.KEK)
	if err != nil {
		return err
	}
	d.DEK, err = d.KeySector.DEK()
	if err != nil {
		return err
	}
	return nil
}

func (d *Decrypter) ExtractDEK(a *kek.Asker) (err error) {
	if !d.Bridge.NeedsKEK() {
		return d.decryptKeySector()
	}

	for a.Ask() {
		d.KEK = a.KEK()
		err = d.decryptKeySector()
		if err == bridge.ErrWrongKEK {
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	// preserve bridge.ErrWrongKEK if we asked to use a specific KEK or used -askonce
	wrong := err == bridge.ErrWrongKEK
	// but return this error first
	if err := a.Err(); err != nil {
		return err
	}
	if wrong {
		return bridge.ErrWrongKEK
	}
	return nil
}

func (d *Decrypter) DecryptDisk() error {
	cipher, err := aes.NewCipher(d.DEK)
	if err != nil {
		return err
	}
	// TODO refine or allow custom buffer sizes?
	iter, err := d.Disk.Iter(0, 1)
	if err != nil {
		return err
	}
	for iter.Next() {
		s := iter.Sectors()
		d.Bridge.Decrypt(cipher, s)
		_, err = d.Out.Write(s)
		if err != nil {
			return err
		}
	}
	return iter.Err()
}