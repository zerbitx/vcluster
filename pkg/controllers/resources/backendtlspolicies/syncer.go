package backendtlspolicies

import (
	"fmt"

	"github.com/loft-sh/vcluster/pkg/mappings"
	"github.com/loft-sh/vcluster/pkg/patcher"
	"github.com/loft-sh/vcluster/pkg/pro"
	"github.com/loft-sh/vcluster/pkg/syncer"
	"github.com/loft-sh/vcluster/pkg/syncer/synccontext"
	"github.com/loft-sh/vcluster/pkg/syncer/translator"
	syncertypes "github.com/loft-sh/vcluster/pkg/syncer/types"
	"github.com/loft-sh/vcluster/pkg/util/translate"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type backendTLSPolicySyncer struct {
	syncertypes.GenericTranslator
	syncertypes.Importer
}

var (
	_ syncertypes.Object          = &backendTLSPolicySyncer{}
	_ syncertypes.Syncer          = &backendTLSPolicySyncer{}
	_ syncertypes.OptionsProvider = &backendTLSPolicySyncer{}
)

func New(ctx *synccontext.RegisterContext) (syncertypes.Object, error) {
	return NewSyncer(ctx)
}

func NewSyncer(ctx *synccontext.RegisterContext) (syncertypes.Object, error) {
	mapper, err := ctx.Mappings.ByGVK(mappings.BackendTLSPolicies())
	if err != nil {
		return nil, err
	}

	return &backendTLSPolicySyncer{
		GenericTranslator: translator.NewGenericTranslator(ctx, "backendtlspolicy", &gatewayv1.BackendTLSPolicy{}, mapper),
		Importer:          pro.NewImporter(mapper),
	}, nil
}

func (s *backendTLSPolicySyncer) Syncer() syncertypes.Sync[client.Object] {
	return syncer.ToGenericSyncer[*gatewayv1.BackendTLSPolicy](s)
}

func (s *backendTLSPolicySyncer) Options() *syncertypes.Options {
	return &syncertypes.Options{
		ObjectCaching: true,
	}
}

func (s *backendTLSPolicySyncer) SyncToHost(ctx *synccontext.SyncContext, event *synccontext.SyncToHostEvent[*gatewayv1.BackendTLSPolicy]) (ctrl.Result, error) {
	if event.HostOld != nil || event.Virtual.DeletionTimestamp != nil {
		return patcher.DeleteVirtualObject(ctx, event.Virtual, event.HostOld, "host object was deleted")
	}

	pObj := translate.HostMetadata(event.Virtual, s.VirtualToHost(ctx, types.NamespacedName{Name: event.Virtual.Name, Namespace: event.Virtual.Namespace}, event.Virtual))

	err := pro.ApplyPatchesHostObject(ctx, nil, pObj, event.Virtual, nil, false)
	if err != nil {
		return ctrl.Result{}, err
	}

	return patcher.CreateHostObject(ctx, event.Virtual, pObj, s.EventRecorder(), true)
}

func (s *backendTLSPolicySyncer) Sync(ctx *synccontext.SyncContext, event *synccontext.SyncEvent[*gatewayv1.BackendTLSPolicy]) (_ ctrl.Result, retErr error) {
	patch, err := patcher.NewSyncerPatcher(ctx, event.Host, event.Virtual, patcher.TranslatePatches(nil, false))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("new syncer patcher: %w", err)
	}

	defer func() {
		if err := patch.Patch(ctx, event.Host, event.Virtual); err != nil {
			retErr = utilerrors.NewAggregate([]error{retErr, err})
		}
		if retErr != nil {
			s.EventRecorder().Eventf(
				event.Virtual,
				nil,
				"Warning",
				"SyncError",
				fmt.Sprintf("Sync%s", event.Virtual.GetObjectKind().GroupVersionKind().Kind),
				"Error syncing: %v",
				retErr,
			)
		}
	}()

	event.Virtual.Status = event.Host.Status
	event.Virtual.Labels, event.Host.Labels = translate.LabelsBidirectionalUpdate(event)
	event.Virtual.Annotations, event.Host.Annotations = translate.AnnotationsBidirectionalUpdate(event)
	event.Host.Spec = event.Virtual.Spec

	return ctrl.Result{}, nil
}

func (s *backendTLSPolicySyncer) SyncToVirtual(ctx *synccontext.SyncContext, event *synccontext.SyncToVirtualEvent[*gatewayv1.BackendTLSPolicy]) (ctrl.Result, error) {
	if event.VirtualOld != nil || translate.ShouldDeleteHostObject(event.Host) {
		return patcher.DeleteHostObject(ctx, event.Host, event.VirtualOld, "virtual object was deleted")
	}

	vPolicy := translate.VirtualMetadata(event.Host, s.HostToVirtual(ctx, types.NamespacedName{Name: event.Host.Name, Namespace: event.Host.Namespace}, event.Host))
	err := pro.ApplyPatchesVirtualObject(ctx, nil, vPolicy, event.Host, nil, false)
	if err != nil {
		return ctrl.Result{}, err
	}

	return patcher.CreateVirtualObject(ctx, event.Host, vPolicy, s.EventRecorder(), true)
}
