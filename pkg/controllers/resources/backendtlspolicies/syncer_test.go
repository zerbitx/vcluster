package backendtlspolicies

import (
	"testing"

	"github.com/loft-sh/vcluster/pkg/config"
	"github.com/loft-sh/vcluster/pkg/scheme"
	"github.com/loft-sh/vcluster/pkg/syncer/synccontext"
	syncertesting "github.com/loft-sh/vcluster/pkg/syncer/testing"
	testingutil "github.com/loft-sh/vcluster/pkg/util/testing"
	"github.com/loft-sh/vcluster/pkg/util/translate"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	testPolicyName      = "testpolicy"
	testPolicyNamespace = "test"
	testServiceName     = "testservice"
	testConfigMapName   = "ca-bundle"
	testControllerName  = gatewayv1.GatewayController("example.com/gateway-controller")
)

func TestSync(t *testing.T) {
	vPolicy := backendTLSPolicy(virtualPolicyMeta(), backendTLSPolicySpec())
	syncCtx, syncer := startBackendTLSPolicySyncer(t, []runtime.Object{
		managedHostService(testServiceName, testPolicyNamespace),
		managedHostConfigMap(testConfigMapName, testPolicyNamespace),
	}, []runtime.Object{vPolicy.DeepCopy()})

	_, err := syncer.SyncToHost(syncCtx, synccontext.NewSyncToHostEvent(vPolicy.DeepCopy()))
	assert.NilError(t, err)

	pName := syncer.VirtualToHost(syncCtx, types.NamespacedName{Name: vPolicy.Name, Namespace: vPolicy.Namespace}, vPolicy)
	storedHost := &gatewayv1.BackendTLSPolicy{}
	err = syncCtx.HostClient.Get(syncCtx, pName, storedHost)
	assert.NilError(t, err)
	assert.DeepEqual(t, storedHost.Spec, hostBackendTLSPolicySpec())

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

	err = syncCtx.HostClient.Get(syncCtx, pName, storedHost)
	assert.NilError(t, err)
	assert.DeepEqual(t, storedHost.Spec, hostBackendTLSPolicySpec())
}

func TestSyncRejectsUnsyncedTargetRef(t *testing.T) {
	vPolicy := backendTLSPolicy(virtualPolicyMeta(), backendTLSPolicySpec())
	syncCtx, syncer := startBackendTLSPolicySyncer(t, []runtime.Object{
		managedHostConfigMap(testConfigMapName, testPolicyNamespace),
	}, []runtime.Object{vPolicy.DeepCopy()})

	_, err := syncer.SyncToHost(syncCtx, synccontext.NewSyncToHostEvent(vPolicy.DeepCopy()))
	assert.ErrorContains(t, err, `referenced Service "testservice" in namespace "test" has no synced host object`)

	storedHost := &gatewayv1.BackendTLSPolicy{}
	err = syncCtx.HostClient.Get(syncCtx, syncer.VirtualToHost(syncCtx, types.NamespacedName{Name: vPolicy.Name, Namespace: vPolicy.Namespace}, vPolicy), storedHost)
	assert.Assert(t, apierrors.IsNotFound(err))
}

func TestSyncRejectsUnsyncedCACertificateRef(t *testing.T) {
	vPolicy := backendTLSPolicy(virtualPolicyMeta(), backendTLSPolicySpec())
	syncCtx, syncer := startBackendTLSPolicySyncer(t, []runtime.Object{
		managedHostService(testServiceName, testPolicyNamespace),
	}, []runtime.Object{vPolicy.DeepCopy()})

	_, err := syncer.SyncToHost(syncCtx, synccontext.NewSyncToHostEvent(vPolicy.DeepCopy()))
	assert.ErrorContains(t, err, `referenced ConfigMap "ca-bundle" in namespace "test" has no synced host object`)
}

func TestSyncSkipsReferenceValidationOnUpdate(t *testing.T) {
	vPolicy := backendTLSPolicy(virtualPolicyMeta(), backendTLSPolicySpec())
	pPolicy := backendTLSPolicy(hostPolicyMeta(), gatewayv1.BackendTLSPolicySpec{})
	syncCtx, syncer := startBackendTLSPolicySyncer(t, []runtime.Object{pPolicy.DeepCopy()}, []runtime.Object{vPolicy.DeepCopy()})

	pPolicy.ResourceVersion = "999"
	vPolicy.ResourceVersion = "999"
	_, err := syncer.Sync(syncCtx, synccontext.NewSyncEventWithOld(pPolicy.DeepCopy(), pPolicy.DeepCopy(), vPolicy.DeepCopy(), vPolicy.DeepCopy()))
	assert.NilError(t, err)

	storedHost := &gatewayv1.BackendTLSPolicy{}
	err = syncCtx.HostClient.Get(syncCtx, types.NamespacedName{Name: pPolicy.Name, Namespace: pPolicy.Namespace}, storedHost)
	assert.NilError(t, err)
	assert.DeepEqual(t, storedHost.Spec, hostBackendTLSPolicySpec())
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

func hostPolicyMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      hostName(testPolicyName, testPolicyNamespace),
		Namespace: hostNamespace(testPolicyNamespace),
	}
}

func backendTLSPolicySpec() gatewayv1.BackendTLSPolicySpec {
	hostname := gatewayv1.PreciseHostname("backend.example.com")
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
			CACertificateRefs: []gatewayv1.LocalObjectReference{
				{
					Group: gatewayv1.Group(""),
					Kind:  gatewayv1.Kind("ConfigMap"),
					Name:  gatewayv1.ObjectName(testConfigMapName),
				},
			},
			Hostname: hostname,
		},
	}
}

func hostBackendTLSPolicySpec() gatewayv1.BackendTLSPolicySpec {
	spec := backendTLSPolicySpec()
	ret := *spec.DeepCopy()
	ret.TargetRefs[0].Name = gatewayv1.ObjectName(hostName(testServiceName, testPolicyNamespace))
	ret.Validation.CACertificateRefs[0].Name = gatewayv1.ObjectName(hostName(testConfigMapName, testPolicyNamespace))
	return ret
}

func managedHostService(name, namespace string) *corev1.Service {
	return translate.HostMetadata(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}, types.NamespacedName{
		Name:      hostName(name, namespace),
		Namespace: hostNamespace(namespace),
	})
}

func managedHostConfigMap(name, namespace string) *corev1.ConfigMap {
	return translate.HostMetadata(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}, types.NamespacedName{
		Name:      hostName(name, namespace),
		Namespace: hostNamespace(namespace),
	})
}

func hostName(name, namespace string) string {
	return translate.SingleNamespaceHostName(name, namespace, translate.VClusterName)
}

func hostNamespace(namespace string) string {
	if namespace == "" {
		return ""
	}

	return testingutil.DefaultTestTargetNamespace
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
