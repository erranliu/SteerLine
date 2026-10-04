package backend

type Command struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
}
