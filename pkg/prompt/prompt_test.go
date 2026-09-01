package prompt

import (
	"bytes"
	"testing"
)

func TestConfirm(t *testing.T) {
	t.Run("User confirms that wants to continue", func(t *testing.T) {
		answers := [][]byte{
			[]byte("Y\n"),
			[]byte("y\n"),
			[]byte("\n"),
		}

		for _, answer := range answers {
			stdin = bytes.NewReader(answer)
			err := Confirm("Do you want to continue?")
			if err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("User does not want to continue", func(t *testing.T) {
		answer := []byte("n\n")
		stdin = bytes.NewReader(answer)
		err := Confirm("Do you want to continue?")
		if err == nil {
			t.Fail()
		}
	})

	t.Run("Default no requires explicit confirmation", func(t *testing.T) {
		stdin = bytes.NewReader([]byte("\n"))
		if err := ConfirmDefaultNo("Trust this app?"); err == nil {
			t.Fatal("expected empty response to decline")
		}

		stdin = bytes.NewReader([]byte("y\n"))
		if err := ConfirmDefaultNo("Trust this app?"); err != nil {
			t.Fatal(err)
		}
	})
}
