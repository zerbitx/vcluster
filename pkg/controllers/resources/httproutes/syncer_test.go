package httproutes

import (
	"testing"

	"github.com/loft-sh/vcluster/pkg/config"
	"github.com/loft-sh/vcluster/pkg/mappings"
	"github.com/loft-sh/vcluster/pkg/scheme"
	"github.com/loft-sh/vcluster/pkg/syncer/synccontext"
	syncertesting "github.com/loft-sh/vcluster/pkg/syncer/testing"
	testingutil "github.com/loft-sh/vcluster/pkg/util/testing"
	"github.com/loft-sh/vcluster/pkg/util/translate"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	testRouteName        = "testroute"
	testRouteNamespace   = "test"
	testGatewayName      = "testgateway"
	testServiceName      = "testservice"
	testMirrorService    = "mirrorservice"
	testAuthService      = "authservice"
	testControllerName   = gatewayv1.GatewayController("example.com/gateway-controller")
	testUnsupportedGroup = gatewayv1.Group("example.com")
)

func TestSync(t *testing.T) {
	vBaseSpec := routeSpec()
	pBaseSpec := hostRouteSpec()
	hostStatus := hostRouteStatus()
	virtualStatus := virtualRouteStatus()
	vObjectMeta := virtualRouteMeta()
	pObjectMeta := hostRouteMeta()
	baseRoute := httpRoute(vObjectMeta, vBaseSpec)
	createdRoute := httpRoute(pObjectMeta, pBaseSpec)
	hostRouteWithStatus := httpRoute(pObjectMeta, gatewayv1.HTTPRouteSpec{}, withStatus(hostStatus))
	expectedHostRouteWithStatus := httpRoute(pObjectMeta, pBaseSpec, withStatus(hostStatus))
	expectedVirtualRouteWithStatus := httpRoute(vObjectMeta, vBaseSpec, withStatus(virtualStatus))

	syncertesting.RunTestsWithContext(t, newHTTPRouteRegisterContext, []*syncertesting.SyncTest{
		{
			Name:                "Create forward",
			InitialVirtualState: []runtime.Object{baseRoute.DeepCopy()},
			ExpectedVirtualState: map[schema.GroupVersionKind][]runtime.Object{
				mappings.HTTPRoutes(): {baseRoute.DeepCopy()},
			},
			ExpectedPhysicalState: map[schema.GroupVersionKind][]runtime.Object{
				mappings.HTTPRoutes(): {createdRoute.DeepCopy()},
			},
			Sync: func(registerContext *synccontext.RegisterContext) {
				syncCtx, syncer := syncertesting.FakeStartSyncer(t, registerContext, NewSyncer)
				_, err := syncer.(*httpRouteSyncer).SyncToHost(syncCtx, synccontext.NewSyncToHostEvent(baseRoute.DeepCopy()))
				assert.NilError(t, err)
			},
		},
		{
			Name:                 "Update forward and status back",
			InitialVirtualState:  []runtime.Object{baseRoute.DeepCopy(), virtualGateway()},
			InitialPhysicalState: []runtime.Object{hostRouteWithStatus.DeepCopy()},
			ExpectedVirtualState: map[schema.GroupVersionKind][]runtime.Object{
				mappings.HTTPRoutes(): {expectedVirtualRouteWithStatus.DeepCopy()},
			},
			ExpectedPhysicalState: map[schema.GroupVersionKind][]runtime.Object{
				mappings.HTTPRoutes(): {expectedHostRouteWithStatus.DeepCopy()},
			},
			Sync: func(registerContext *synccontext.RegisterContext) {
				syncCtx, syncer := syncertesting.FakeStartSyncer(t, registerContext, NewSyncer)
				pRoute := hostRouteWithStatus.DeepCopy()
				pRoute.ResourceVersion = "999"
				vRoute := baseRoute.DeepCopy()
				vRoute.ResourceVersion = "999"

				_, err := syncer.(*httpRouteSyncer).Sync(syncCtx, synccontext.NewSyncEventWithOld(pRoute, pRoute, vRoute, vRoute))
				assert.NilError(t, err)
			},
		},
	})
}

func TestSyncRejectsUnsupportedRefs(t *testing.T) {
	tests := []struct {
		name        string
		route       *gatewayv1.HTTPRoute
		expectedErr string
	}{
		{
			name: "Unsupported parentRef",
			route: httpRoute(virtualRouteMeta(), gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{
						{
							Group: ptr.To(testUnsupportedGroup),
							Kind:  ptr.To(gatewayv1.Kind("ExampleGateway")),
							Name:  gatewayv1.ObjectName(testGatewayName),
						},
					},
				},
			}),
			expectedErr: `parentRef group "example.com" kind "ExampleGateway" is not supported`,
		},
		{
			name: "Unsupported backendRef",
			route: httpRoute(virtualRouteMeta(), gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(testGatewayName)}},
				},
				Rules: []gatewayv1.HTTPRouteRule{
					{
						BackendRefs: []gatewayv1.HTTPBackendRef{
							{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Group: ptr.To(testUnsupportedGroup),
										Kind:  ptr.To(gatewayv1.Kind("ExampleBackend")),
										Name:  gatewayv1.ObjectName(testServiceName),
									},
								},
							},
						},
					},
				},
			}),
			expectedErr: `backendRef group "example.com" kind "ExampleBackend" is not supported`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			syncCtx, syncer := startHTTPRouteSyncer(t, nil, []runtime.Object{tc.route}, nil)
			_, err := syncer.SyncToHost(syncCtx, synccontext.NewSyncToHostEvent(tc.route.DeepCopy()))
			assert.ErrorContains(t, err, tc.expectedErr)
		})
	}
}

func newHTTPRouteRegisterContext(vConfig *config.VirtualClusterConfig, pClient *testingutil.FakeIndexClient, vClient *testingutil.FakeIndexClient) *synccontext.RegisterContext {
	vConfig.Sync.ToHost.Gateways.Enabled = true
	vConfig.Sync.ToHost.HTTPRoutes.Enabled = true
	return syncertesting.NewFakeRegisterContext(vConfig, pClient, vClient)
}

func startHTTPRouteSyncer(
	t *testing.T,
	initialPhysicalState []runtime.Object,
	initialVirtualState []runtime.Object,
	adjustConfig func(*config.VirtualClusterConfig),
) (*synccontext.SyncContext, *httpRouteSyncer) {
	t.Helper()

	pClient := testingutil.NewFakeClient(scheme.Scheme, initialPhysicalState...)
	vClient := testingutil.NewFakeClient(scheme.Scheme, initialVirtualState...)
	vConfig := testingutil.NewFakeConfig()
	if adjustConfig != nil {
		adjustConfig(vConfig)
	}

	registerContext := newHTTPRouteRegisterContext(vConfig, pClient, vClient)
	syncCtx, syncer := syncertesting.FakeStartSyncer(t, registerContext, NewSyncer)
	return syncCtx, syncer.(*httpRouteSyncer)
}

type httpRouteOption func(*gatewayv1.HTTPRoute)

func withStatus(status gatewayv1.HTTPRouteStatus) httpRouteOption {
	return func(route *gatewayv1.HTTPRoute) {
		route.Status = status
	}
}

func httpRoute(meta metav1.ObjectMeta, spec gatewayv1.HTTPRouteSpec, opts ...httpRouteOption) *gatewayv1.HTTPRoute {
	ret := &gatewayv1.HTTPRoute{
		ObjectMeta: meta,
		Spec:       spec,
	}
	for _, opt := range opts {
		opt(ret)
	}
	return ret
}

func routeSpec() gatewayv1.HTTPRouteSpec {
	return gatewayv1.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1.CommonRouteSpec{
			ParentRefs: []gatewayv1.ParentReference{
				{Name: gatewayv1.ObjectName(testGatewayName)},
			},
		},
		Hostnames: []gatewayv1.Hostname{"example.com"},
		Rules: []gatewayv1.HTTPRouteRule{
			{
				BackendRefs: []gatewayv1.HTTPBackendRef{
					serviceBackendRef(testServiceName, withBackendRefFilter(mirrorFilter(testMirrorService))),
				},
				Filters: []gatewayv1.HTTPRouteFilter{
					mirrorFilter(testMirrorService),
					externalAuthFilter(testAuthService),
				},
			},
		},
	}
}

func hostRouteSpec() gatewayv1.HTTPRouteSpec {
	spec := routeSpec()
	ret := *spec.DeepCopy()
	ret.ParentRefs[0].Name = gatewayv1.ObjectName(hostName(testGatewayName))
	ret.Rules[0].BackendRefs[0].Name = gatewayv1.ObjectName(hostName(testServiceName))
	ret.Rules[0].BackendRefs[0].Filters[0].RequestMirror.BackendRef.Name = gatewayv1.ObjectName(hostName(testMirrorService))
	ret.Rules[0].Filters[0].RequestMirror.BackendRef.Name = gatewayv1.ObjectName(hostName(testMirrorService))
	ret.Rules[0].Filters[1].ExternalAuth.BackendRef.Name = gatewayv1.ObjectName(hostName(testAuthService))
	return ret
}

type backendRefOption func(*gatewayv1.HTTPBackendRef)

func withBackendRefFilter(filter gatewayv1.HTTPRouteFilter) backendRefOption {
	return func(ref *gatewayv1.HTTPBackendRef) {
		ref.Filters = append(ref.Filters, filter)
	}
}

func serviceBackendRef(name string, opts ...backendRefOption) gatewayv1.HTTPBackendRef {
	ret := gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName(name),
				Port: ptr.To(gatewayv1.PortNumber(80)),
			},
		},
	}
	for _, opt := range opts {
		opt(&ret)
	}
	return ret
}

func mirrorFilter(serviceName string) gatewayv1.HTTPRouteFilter {
	return gatewayv1.HTTPRouteFilter{
		Type: gatewayv1.HTTPRouteFilterRequestMirror,
		RequestMirror: &gatewayv1.HTTPRequestMirrorFilter{
			BackendRef: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName(serviceName),
				Port: ptr.To(gatewayv1.PortNumber(80)),
			},
		},
	}
}

func externalAuthFilter(serviceName string) gatewayv1.HTTPRouteFilter {
	return gatewayv1.HTTPRouteFilter{
		Type: gatewayv1.HTTPRouteFilterExternalAuth,
		ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
			ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthHTTPProtocol,
			BackendRef: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName(serviceName),
				Port: ptr.To(gatewayv1.PortNumber(80)),
			},
		},
	}
}

func virtualRouteStatus() gatewayv1.HTTPRouteStatus {
	status := hostRouteStatus()
	status.Parents[0].ParentRef.Name = gatewayv1.ObjectName(testGatewayName)
	return status
}

func hostRouteStatus() gatewayv1.HTTPRouteStatus {
	return gatewayv1.HTTPRouteStatus{
		RouteStatus: gatewayv1.RouteStatus{
			Parents: []gatewayv1.RouteParentStatus{
				{
					ParentRef:      gatewayv1.ParentReference{Name: gatewayv1.ObjectName(hostName(testGatewayName))},
					ControllerName: testControllerName,
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gatewayv1.RouteReasonAccepted),
						},
					},
				},
			},
		},
	}
}

func virtualRouteMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      testRouteName,
		Namespace: testRouteNamespace,
	}
}

func hostRouteMeta() metav1.ObjectMeta {
	hostRouteName := hostName(testRouteName)
	return metav1.ObjectMeta{
		Name:      hostRouteName,
		Namespace: testRouteNamespace,
		Annotations: map[string]string{
			translate.NameAnnotation:          testRouteName,
			translate.NamespaceAnnotation:     testRouteNamespace,
			translate.UIDAnnotation:           "",
			translate.KindAnnotation:          mappings.HTTPRoutes().String(),
			translate.HostNamespaceAnnotation: testRouteNamespace,
			translate.HostNameAnnotation:      hostRouteName,
		},
		Labels: map[string]string{
			translate.MarkerLabel:    translate.VClusterName,
			translate.NamespaceLabel: testRouteNamespace,
		},
		ResourceVersion: "999",
	}
}

func hostName(name string) string {
	return translate.Default.HostName(nil, name, testRouteNamespace).Name
}

func virtualGateway() *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testGatewayName,
			Namespace: testRouteNamespace,
		},
	}
}
