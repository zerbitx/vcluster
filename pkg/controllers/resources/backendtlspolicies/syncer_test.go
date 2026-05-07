package backendtlspolicies

import (
	"testing"

	"github.com/loft-sh/vcluster/pkg/config"
	"github.com/loft-sh/vcluster/pkg/scheme"
	"github.com/loft-sh/vcluster/pkg/syncer/synccontext"
	syncertesting "github.com/loft-sh/vcluster/pkg/syncer/testing"
	testingutil "github.com/loft-sh/vcluster/pkg/util/testing"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	testPolicyName      = "testpolicy"
	testPolicyNamespace = "test"
	testServiceName     = "testservice"
	testControllerName  = gatewayv1.GatewayController("example.com/gateway-controller")
)

func TestSync(t *testing.T) {
	vPolicy := backendTLSPolicy(virtualPolicyMeta(), backendTLSPolicySpec())
	syncCtx, syncer := startBackendTLSPolicySyncer(t, nil, []runtime.Object{vPolicy.DeepCopy()})

	_, err := syncer.SyncToHost(syncCtx, synccontext.NewSyncToHostEvent(vPolicy.DeepCopy()))
	assert.NilError(t, err)

	pName := syncer.VirtualToHost(syncCtx, types.NamespacedName{Name: vPolicy.Name, Namespace: vPolicy.Namespace}, vPolicy)
	storedHost := &gatewayv1.BackendTLSPolicy{}
	err = syncCtx.HostClient.Get(syncCtx, pName, storedHost)
	assert.NilError(t, err)
	assert.DeepEqual(t, storedHost.Spec, vPolicy.Spec)

	hostStatus := backendTLSPolicyStatus()
	storedHost.Status = hostStatus
	storedHost.ResourceVersion = "999"
	vPolicy.ResourceVersion = "999"

	_, err = syncer.Sync(syncCtx, synccontext.NewSyncEventWithOld(storedHost.DeepCopy(), storedHost.DeepCopy(), vPolicy.DeepCopy(), vPolicy.DeepCopy()))
	assert.NilError(t, err)

	storedVirtual := &gatewayv1.BackendTLSPolicy{}
	err = syncCtx.VirtualClient.Get(syncCtx, types.NamespacedName{Name: vPolicy.Name, Namespace: vPolicy.Namespace}, storedVirtual)
	assert.NilError(t, err)
	assert.DeepEqual(t, storedVirtual.Status, hostStatus)
}

func startBackendTLSPolicySyncer(
	t *testing.T,
	initialPhysicalState []runtime.Object,
	initialVirtualState []runtime.Object,
) (*synccontext.SyncContext, *backendTLSPolicySyncer) {
	t.Helper()

	pClient := testingutil.NewFakeClient(scheme.Scheme, initialPhysicalState...)
	vClient := testingutil.NewFakeClient(scheme.Scheme, initialVirtualState...)
	vConfig := testingutil.NewFakeConfig()
	registerContext := newBackendTLSPolicyRegisterContext(vConfig, pClient, vClient)
	syncCtx, syncer := syncertesting.FakeStartSyncer(t, registerContext, NewSyncer)
	return syncCtx, syncer.(*backendTLSPolicySyncer)
}

func newBackendTLSPolicyRegisterContext(vConfig *config.VirtualClusterConfig, pClient *testingutil.FakeIndexClient, vClient *testingutil.FakeIndexClient) *synccontext.RegisterContext {
	vConfig.Sync.ToHost.Gateways.Enabled = true
	return syncertesting.NewFakeRegisterContext(vConfig, pClient, vClient)
}

func backendTLSPolicy(meta metav1.ObjectMeta, spec gatewayv1.BackendTLSPolicySpec) *gatewayv1.BackendTLSPolicy {
	return &gatewayv1.BackendTLSPolicy{
		ObjectMeta: meta,
		Spec:       spec,
	}
}

func virtualPolicyMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      testPolicyName,
		Namespace: testPolicyNamespace,
	}
}

func backendTLSPolicySpec() gatewayv1.BackendTLSPolicySpec {
	hostname := gatewayv1.PreciseHostname("backend.example.com")
	wellKnownCA := gatewayv1.WellKnownCACertificatesType("System")
	return gatewayv1.BackendTLSPolicySpec{
		TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
			{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Group: gatewayv1.Group(""),
					Kind:  gatewayv1.Kind("Service"),
					Name:  gatewayv1.ObjectName(testServiceName),
				},
			},
		},
		Validation: gatewayv1.BackendTLSPolicyValidation{
			WellKnownCACertificates: &wellKnownCA,
			Hostname:                hostname,
		},
	}
}

func backendTLSPolicyStatus() gatewayv1.PolicyStatus {
	return gatewayv1.PolicyStatus{
		Ancestors: []gatewayv1.PolicyAncestorStatus{
			{
				AncestorRef: gatewayv1.ParentReference{
					Name: gatewayv1.ObjectName("host-gateway"),
				},
				ControllerName: testControllerName,
				Conditions: []metav1.Condition{
					{
						Type:   string(gatewayv1.PolicyConditionAccepted),
						Status: metav1.ConditionTrue,
						Reason: string(gatewayv1.PolicyReasonAccepted),
					},
				},
			},
		},
	}
}
