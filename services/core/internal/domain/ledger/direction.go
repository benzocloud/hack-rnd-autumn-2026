package ledger

type Direction string

const (
	Inflow  Direction = "inflow"
	Outflow Direction = "outflow"
)

func (d Direction) Validate() error {
	if d != Inflow && d != Outflow {
		return ErrInvalid
	}
	return nil
}
