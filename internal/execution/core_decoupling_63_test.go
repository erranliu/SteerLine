package execution

import "testing"

func TestCD63FrozenConfigAndBinding(t *testing.T) {
	t.Run("recovery_seed_resources", testCD63RecoverySeedResources)
}
