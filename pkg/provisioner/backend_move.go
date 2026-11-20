// The backend move methods of the Provisioner move terraform state after terraform.backend.type changes.
// They find the components whose state is still in the previous backend, then move and verify that state.
// The plan methods use them to plan each component against the backend that holds its state.

package provisioner

import (
	"context"
	"fmt"
	"slices"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	terraforminfra "github.com/windsorcli/cli/pkg/provisioner/terraform"
)

// =============================================================================
// Types
// =============================================================================

// BackendMove describes a pending change of the terraform backend type. To is the configured backend
// type. Components lists the components with a pending move, in blueprint order.
type BackendMove struct {
	To         string
	Components []ComponentMove
}

// ComponentMove names one component with a pending move and the backend type that holds its state.
type ComponentMove struct {
	ID   string
	From string
}

// =============================================================================
// Public Methods
// =============================================================================

// PendingBackendMove returns the enabled components whose terraform state is in another backend than
// the configured one. A component has a pending move when its backend pointer names another backend
// type and that backend holds its state. Local state with no pointer also counts as a move from local.
// It reads files only and runs no terraform command. It returns nil when no move is pending.
func (i *Provisioner) PendingBackendMove(blueprint *blueprintv1alpha1.Blueprint) (*BackendMove, error) {
	if blueprint == nil {
		return nil, fmt.Errorf("blueprint not provided")
	}
	if err := i.ensureTerraformStack(); err != nil {
		return nil, err
	}
	if i.TerraformStack == nil {
		return nil, nil
	}
	to := i.configuredBackendType()
	var move *BackendMove
	for _, c := range blueprint.TerraformComponents {
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		from, err := i.pendingMoveSource(c.GetID(), to)
		if err != nil {
			return nil, err
		}
		if from == "" {
			continue
		}
		if move == nil {
			move = &BackendMove{To: to}
		}
		move.Components = append(move.Components, ComponentMove{ID: c.GetID(), From: from})
	}
	return move, nil
}

// Sources returns each backend type that holds the state of a pending component, once, in the
// order of the first component that uses it.
func (m *BackendMove) Sources() []string {
	var sources []string
	for _, c := range m.Components {
		if !slices.Contains(sources, c.From) {
			sources = append(sources, c.From)
		}
	}
	return sources
}

// =============================================================================
// Private Methods
// =============================================================================

// configuredBackendType returns the configured terraform backend type, or local when none is set.
func (i *Provisioner) configuredBackendType() string {
	if backendType := i.configHandler.GetTerraformBackendType(); backendType != "" {
		return backendType
	}
	return "local"
}

// pendingMoveSource returns the backend type that holds the state of the component when it is not to.
// It returns "" when no move is pending. A local pointer, or no pointer, counts only when the local
// state file has resources. The none backend type never takes part in a move.
func (i *Provisioner) pendingMoveSource(componentID, to string) (string, error) {
	if to == "none" {
		return "", nil
	}
	pointer, err := i.TerraformStack.BackendPointerType(componentID)
	if err != nil {
		return "", err
	}
	if pointer == to || pointer == "none" {
		return "", nil
	}
	if pointer != "" && pointer != "local" {
		return pointer, nil
	}
	if to == "local" {
		return "", nil
	}
	hasLocal, err := i.TerraformStack.HasLocalStateWithResources(componentID)
	if err != nil {
		return "", werror.Wrap(err, "error inspecting local state for %s", componentID)
	}
	if !hasLocal {
		return "", nil
	}
	return "local", nil
}

// checkBackendMoveReachable returns an error when the move of any component reads or writes the
// kubernetes backend and Windsor cannot reach the cluster. It ignores the backend components that
// applyWithBackendPivot moves itself, because the pivot creates the backend before it moves them.
// It runs before any change, so a refused run changes nothing.
func (i *Provisioner) checkBackendMoveReachable(blueprint *blueprintv1alpha1.Blueprint) error {
	move, err := i.PendingBackendMove(blueprint)
	if err != nil || move == nil {
		return err
	}
	tier := i.backendTierIDs(blueprint)
	for _, c := range move.Components {
		if slices.Contains(tier, c.ID) || (c.From != "kubernetes" && move.To != "kubernetes") {
			continue
		}
		if i.clusterReachable() {
			return nil
		}
		return unreachableMoveError(c.From, move.To)
	}
	return nil
}

// moveBackendState moves the state of each enabled component with a pending move to the configured
// backend, as moveComponentState describes. A component with no backend pointer moves only when the
// configured backend holds no state for it. A component whose pointer already names the configured
// backend but still has a local state file finishes an interrupted move, as finishMove describes.
// A failed probe of the configured backend returns an error, because -force-copy can overwrite state.
func (i *Provisioner) moveBackendState(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	to := i.configuredBackendType()
	if to == "none" {
		return nil
	}
	movedFromKubernetes := false
	for _, c := range blueprint.TerraformComponents {
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		componentID := c.GetID()
		pointer, err := i.TerraformStack.BackendPointerType(componentID)
		if err != nil {
			return err
		}
		from, err := i.pendingMoveSource(componentID, to)
		if err != nil {
			return err
		}
		switch {
		case from != "" && pointer == "":
			hasRemote, err := i.HasRemoteState(blueprint, componentID)
			if err != nil {
				return werror.New(fmt.Sprintf("could not read the %s backend for %q: %v", to, componentID, err), "Check the credentials, the network connection, and the backend storage, then run the command again. Windsor stops because a state move can overwrite valid state.", err)
			}
			if hasRemote {
				continue
			}
			if err := i.moveComponentState(ctx, blueprint, componentID, from, to); err != nil {
				return err
			}
		case from != "":
			if err := i.moveComponentState(ctx, blueprint, componentID, from, to); err != nil {
				return err
			}
			movedFromKubernetes = movedFromKubernetes || from == "kubernetes"
		case to != "local" && pointer == to:
			if err := i.finishMove(ctx, blueprint, componentID, to); err != nil {
				return err
			}
		}
	}
	if movedFromKubernetes {
		presenter.Message(ctx, "The terraform state Secrets stay in the cluster. Windsor does not delete them.")
	}
	return nil
}

// moveComponentState moves the state of one component from the from backend to the to backend with
// terraform init -migrate-state -force-copy. It reads the source state first and the copy after, and
// returns an error when the copy does not match, as stateCopyMatches describes. It removes the local
// state file only after the copy matches. It does nothing when the source backend holds no state.
func (i *Provisioner) moveComponentState(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID, from, to string) error {
	message := fmt.Sprintf("Moving terraform state of %s from %s to %s", componentID, from, to)
	return presenter.Track(ctx, "terraform:"+componentID, message, func(ctx context.Context) error {
		source, err := i.stateSnapshotIn(ctx, blueprint, componentID, from)
		if err != nil {
			return werror.Wrap(err, "error reading the %s state of %s", from, componentID)
		}
		if source == nil {
			return nil
		}
		if from == "local" {
			if err := i.withBackendOverride(ctx, "local-move-init", func() error {
				return i.InitComponent(blueprint, componentID)
			}); err != nil {
				return werror.Wrap(err, "error resetting backend pointer")
			}
		}
		if err := i.MigrateComponentState(blueprint, componentID); err != nil {
			return err
		}
		if err := i.verifyStateCopy(ctx, blueprint, componentID, source, from, to); err != nil {
			return err
		}
		if from == "local" {
			if err := i.RemoveLocalState(componentID); err != nil {
				presenter.Warn(ctx, fmt.Sprintf("failed to remove local state file for %q after the move: %v", componentID, err))
			}
		}
		return nil
	})
}

// finishMove removes the local state file of a component whose backend pointer already names the
// to backend. An interrupted move leaves this file. It removes the file only when the state in the to
// backend matches it, as stateCopyMatches describes. Otherwise it keeps the file and warns.
func (i *Provisioner) finishMove(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID, to string) error {
	local, err := i.TerraformStack.LocalStateSnapshot(componentID)
	if err != nil {
		return err
	}
	if local == nil || local.Resources == 0 {
		return nil
	}
	remote, err := i.TerraformStack.StateSnapshot(blueprint, componentID)
	if err != nil {
		return werror.Wrap(err, "error reading the %s state of %s", to, componentID)
	}
	if !stateCopyMatches(local, remote) {
		presenter.Warn(ctx, fmt.Sprintf("the local terraform state of %q does not match its state in the %s backend; Windsor keeps the local file. Remove the file when the %s state is correct.", componentID, to, to))
		return nil
	}
	if err := i.RemoveLocalState(componentID); err != nil {
		presenter.Warn(ctx, fmt.Sprintf("failed to remove local state file for %q after the move: %v", componentID, err))
		return nil
	}
	presenter.Message(ctx, fmt.Sprintf("Removed the local terraform state of %s, which the %s backend already holds", componentID, to))
	return nil
}

// verifyStateCopy reads the state of the component in the to backend and returns an error when it
// does not match source, as stateCopyMatches describes. The error gives the lineage, the serial, and
// the resource count of both states, and says when a state is empty. It names the from backend, which
// still holds the state.
func (i *Provisioner) verifyStateCopy(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID string, source *terraforminfra.StateSnapshot, from, to string) error {
	copied, err := i.stateSnapshotIn(ctx, blueprint, componentID, to)
	if err != nil {
		return werror.Wrap(err, "error reading the %s state of %s after the move", to, componentID)
	}
	if !stateCopyMatches(source, copied) {
		return werror.New(
			fmt.Sprintf("the %s state of %s does not match its %s state after the move (%s; %s)", to, componentID, from, describeState(from, source), describeState(to, copied)),
			fmt.Sprintf("Windsor keeps the %s state. Compare the two states, then run the command again.", from),
			nil,
		)
	}
	return nil
}

// stateSnapshotIn reads the state of the component in the backend type. It reads the local state file
// for local. For another type, it runs terraform state pull with that backend type set.
func (i *Provisioner) stateSnapshotIn(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint, componentID, backendType string) (*terraforminfra.StateSnapshot, error) {
	if backendType == "local" {
		return i.TerraformStack.LocalStateSnapshot(componentID)
	}
	var snapshot *terraforminfra.StateSnapshot
	err := i.withBackendType(ctx, backendType, "state read", func() error {
		var err error
		snapshot, err = i.TerraformStack.StateSnapshot(blueprint, componentID)
		return err
	})
	return snapshot, err
}

// planInStateBackend runs fn once for each run of consecutive components that plan against the same
// backend. A component with a pending move plans against the backend that holds its state, so the
// plan shows the changes that apply makes after the move. Each call gets a copy of the blueprint
// with only the components of its run. A run that reads the kubernetes backend returns an error when
// Windsor cannot reach the cluster.
func (i *Provisioner) planInStateBackend(blueprint *blueprintv1alpha1.Blueprint, fn func(*blueprintv1alpha1.Blueprint) error) error {
	to := i.configuredBackendType()
	type run struct {
		backend    string
		components []blueprintv1alpha1.TerraformComponent
	}
	var runs []run
	for _, c := range blueprint.TerraformComponents {
		backend := ""
		if c.Enabled == nil || c.Enabled.IsEnabled() {
			from, err := i.pendingMoveSource(c.GetID(), to)
			if err != nil {
				return err
			}
			backend = from
		} else if len(runs) > 0 {
			backend = runs[len(runs)-1].backend
		}
		if len(runs) == 0 || runs[len(runs)-1].backend != backend {
			runs = append(runs, run{backend: backend})
		}
		runs[len(runs)-1].components = append(runs[len(runs)-1].components, c)
	}
	if len(runs) <= 1 && (len(runs) == 0 || runs[0].backend == "") {
		return fn(blueprint)
	}
	for _, r := range runs {
		subset := *blueprint
		subset.TerraformComponents = r.components
		if r.backend == "" {
			if err := fn(&subset); err != nil {
				return err
			}
			continue
		}
		if r.backend == "kubernetes" && !i.clusterReachable() {
			return unreachableMoveError(r.backend, to)
		}
		if err := i.withBackendType(context.Background(), r.backend, "plan", func() error {
			return fn(&subset)
		}); err != nil {
			return err
		}
	}
	return nil
}

// planComponentInStateBackend runs fn for one component, against the backend that holds its state
// when the component has a pending move, as planInStateBackend describes.
func (i *Provisioner) planComponentInStateBackend(componentID string, fn func() error) error {
	to := i.configuredBackendType()
	from, err := i.pendingMoveSource(componentID, to)
	if err != nil {
		return err
	}
	if from == "" {
		return fn()
	}
	if from == "kubernetes" && !i.clusterReachable() {
		return unreachableMoveError(from, to)
	}
	return i.withBackendType(context.Background(), from, "plan", fn)
}

// =============================================================================
// Helpers
// =============================================================================

// stateCopyMatches reports whether copied is a full copy of source. The copy must have the same
// lineage and resource count, and a serial no lower than the source. Terraform can raise the serial
// by one when it writes over older state in the destination.
func stateCopyMatches(source, copied *terraforminfra.StateSnapshot) bool {
	if source == nil || copied == nil {
		return false
	}
	return copied.Lineage == source.Lineage && copied.Resources == source.Resources && copied.Serial >= source.Serial
}

// describeState returns the backend type and the lineage, serial, and resource count of snapshot. It
// returns the backend type and "read back empty" for a nil snapshot.
func describeState(backendType string, snapshot *terraforminfra.StateSnapshot) string {
	if snapshot == nil {
		return fmt.Sprintf("%s: read back empty", backendType)
	}
	return fmt.Sprintf("%s: lineage %s, serial %d, %d resources", backendType, snapshot.Lineage, snapshot.Serial, snapshot.Resources)
}

// unreachableMoveError returns the error for a move from the from backend to the to backend that
// needs a cluster Windsor cannot reach.
func unreachableMoveError(from, to string) error {
	return werror.New(
		fmt.Sprintf("Windsor cannot reach the cluster to move terraform state from the %s backend to the %s backend", from, to),
		fmt.Sprintf("Make the cluster reachable, then run the command again. To stop the move, set terraform.backend.type back to %s.", from),
		nil,
	)
}
