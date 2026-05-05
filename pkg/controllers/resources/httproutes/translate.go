package httproutes

import (
	"fmt"

	"github.com/loft-sh/vcluster/pkg/mappings"
	"github.com/loft-sh/vcluster/pkg/syncer/synccontext"
	"github.com/loft-sh/vcluster/pkg/util/translate"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (s *httpRouteSyncer) translate(ctx *synccontext.SyncContext, vRoute *gatewayv1.HTTPRoute) (*gatewayv1.HTTPRoute, error) {
	pRoute := translate.HostMetadata(vRoute, s.VirtualToHost(ctx, types.NamespacedName{Name: vRoute.Name, Namespace: vRoute.Namespace}, vRoute))

	spec, err := translateSpecToHost(ctx, vRoute)
	if err != nil {
		return nil, err
	}

	pRoute.Spec = *spec
	return pRoute, nil
}

func translateSpecToHost(ctx *synccontext.SyncContext, vRoute *gatewayv1.HTTPRoute) (*gatewayv1.HTTPRouteSpec, error) {
	retSpec := vRoute.Spec.DeepCopy()

	for i := range retSpec.ParentRefs {
		err := translateParentRefToHost(ctx, vRoute.Namespace, &retSpec.ParentRefs[i])
		if err != nil {
			return nil, fmt.Errorf("translate parentRefs[%d]: %w", i, err)
		}
	}

	for i := range retSpec.Rules {
		err := translateRuleToHost(ctx, vRoute.Namespace, &retSpec.Rules[i])
		if err != nil {
			return nil, fmt.Errorf("translate rules[%d]: %w", i, err)
		}
	}

	return retSpec, nil
}

func translateStatusToVirtual(ctx *synccontext.SyncContext, hostRouteNamespace, virtualRouteNamespace string, status gatewayv1.HTTPRouteStatus) (gatewayv1.HTTPRouteStatus, error) {
	retStatus := *status.DeepCopy()

	for i := range retStatus.Parents {
		err := translateParentRefToVirtual(ctx, hostRouteNamespace, virtualRouteNamespace, &retStatus.Parents[i].ParentRef)
		if err != nil {
			return gatewayv1.HTTPRouteStatus{}, fmt.Errorf("translate parents[%d].parentRef: %w", i, err)
		}
	}

	return retStatus, nil
}

func translateRuleToHost(ctx *synccontext.SyncContext, routeNamespace string, rule *gatewayv1.HTTPRouteRule) error {
	for i := range rule.BackendRefs {
		err := translateHTTPBackendRefToHost(ctx, routeNamespace, &rule.BackendRefs[i])
		if err != nil {
			return fmt.Errorf("translate backendRefs[%d]: %w", i, err)
		}
	}

	for i := range rule.Filters {
		err := translateFilterToHost(ctx, routeNamespace, &rule.Filters[i])
		if err != nil {
			return fmt.Errorf("translate filters[%d]: %w", i, err)
		}
	}

	return nil
}

func translateHTTPBackendRefToHost(ctx *synccontext.SyncContext, routeNamespace string, ref *gatewayv1.HTTPBackendRef) error {
	err := translateBackendObjectRefToHost(ctx, routeNamespace, &ref.BackendObjectReference)
	if err != nil {
		return err
	}

	for i := range ref.Filters {
		err := translateFilterToHost(ctx, routeNamespace, &ref.Filters[i])
		if err != nil {
			return fmt.Errorf("translate filters[%d]: %w", i, err)
		}
	}

	return nil
}

func translateFilterToHost(ctx *synccontext.SyncContext, routeNamespace string, filter *gatewayv1.HTTPRouteFilter) error {
	if filter.RequestMirror != nil {
		err := translateBackendObjectRefToHost(ctx, routeNamespace, &filter.RequestMirror.BackendRef)
		if err != nil {
			return fmt.Errorf("translate requestMirror.backendRef: %w", err)
		}
	}

	if filter.ExternalAuth != nil {
		err := translateBackendObjectRefToHost(ctx, routeNamespace, &filter.ExternalAuth.BackendRef)
		if err != nil {
			return fmt.Errorf("translate externalAuth.backendRef: %w", err)
		}
	}

	return nil
}

func translateParentRefToHost(ctx *synccontext.SyncContext, routeNamespace string, ref *gatewayv1.ParentReference) error {
	gvk, err := parentReferenceGVK(ref)
	if err != nil {
		return err
	}

	hostName, err := translateRefToHost(ctx, routeNamespace, ref.Name, ref.Namespace, gvk)
	if err != nil {
		return err
	}

	ref.Name = gatewayv1.ObjectName(hostName.Name)
	if ref.Namespace != nil {
		ref.Namespace = ptr.To(gatewayv1.Namespace(hostName.Namespace))
	}

	return nil
}

func translateParentRefToVirtual(ctx *synccontext.SyncContext, hostRouteNamespace, virtualRouteNamespace string, ref *gatewayv1.ParentReference) error {
	gvk, err := parentReferenceGVK(ref)
	if err != nil {
		return err
	}

	virtualName, err := translateRefToVirtual(ctx, hostRouteNamespace, ref.Name, ref.Namespace, gvk)
	if err != nil {
		return err
	}

	ref.Name = gatewayv1.ObjectName(virtualName.Name)
	if virtualName.Namespace != virtualRouteNamespace {
		ref.Namespace = ptr.To(gatewayv1.Namespace(virtualName.Namespace))
	} else {
		ref.Namespace = nil
	}

	return nil
}

func translateBackendObjectRefToHost(ctx *synccontext.SyncContext, routeNamespace string, ref *gatewayv1.BackendObjectReference) error {
	gvk, err := backendReferenceGVK(ref)
	if err != nil {
		return err
	}

	hostName, err := translateRefToHost(ctx, routeNamespace, ref.Name, ref.Namespace, gvk)
	if err != nil {
		return err
	}

	ref.Name = gatewayv1.ObjectName(hostName.Name)
	if ref.Namespace != nil {
		ref.Namespace = ptr.To(gatewayv1.Namespace(hostName.Namespace))
	}

	return nil
}

func translateRefToHost(ctx *synccontext.SyncContext, routeNamespace string, refName gatewayv1.ObjectName, refNamespace *gatewayv1.Namespace, gvk schema.GroupVersionKind) (types.NamespacedName, error) {
	mapper, err := ctx.Mappings.ByGVK(gvk)
	if err != nil {
		return types.NamespacedName{}, err
	}

	hostName := mapper.VirtualToHost(ctx, types.NamespacedName{
		Name:      string(refName),
		Namespace: refNamespaceOrRouteNamespace(routeNamespace, refNamespace),
	}, nil)
	if hostName.Name == "" {
		return types.NamespacedName{}, fmt.Errorf("could not translate virtual %s %q to host", gvk.Kind, refName)
	}

	return hostName, nil
}

func translateRefToVirtual(ctx *synccontext.SyncContext, hostRouteNamespace string, refName gatewayv1.ObjectName, refNamespace *gatewayv1.Namespace, gvk schema.GroupVersionKind) (types.NamespacedName, error) {
	mapper, err := ctx.Mappings.ByGVK(gvk)
	if err != nil {
		return types.NamespacedName{}, err
	}

	virtualName := mapper.HostToVirtual(ctx, types.NamespacedName{
		Name:      string(refName),
		Namespace: refNamespaceOrRouteNamespace(hostRouteNamespace, refNamespace),
	}, nil)
	if virtualName.Name == "" {
		return types.NamespacedName{}, fmt.Errorf("could not translate host %s %q to virtual", gvk.Kind, refName)
	}

	return virtualName, nil
}

func refNamespaceOrRouteNamespace(routeNamespace string, refNamespace *gatewayv1.Namespace) string {
	if refNamespace == nil || *refNamespace == "" {
		return routeNamespace
	}

	return string(*refNamespace)
}

func parentReferenceGVK(ref *gatewayv1.ParentReference) (schema.GroupVersionKind, error) {
	group := gatewayv1.GroupVersion.Group
	if ref.Group != nil {
		group = string(*ref.Group)
	}

	kind := "Gateway"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}

	switch {
	case group == gatewayv1.GroupVersion.Group && kind == "Gateway":
		return mappings.Gateways(), nil
	case group == corev1.GroupName && kind == "Service":
		return mappings.Services(), nil
	default:
		return schema.GroupVersionKind{}, fmt.Errorf("parentRef group %q kind %q is not supported", group, kind)
	}
}

func backendReferenceGVK(ref *gatewayv1.BackendObjectReference) (schema.GroupVersionKind, error) {
	group := corev1.GroupName
	if ref.Group != nil {
		group = string(*ref.Group)
	}

	kind := "Service"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}

	if group == corev1.GroupName && kind == "Service" {
		return mappings.Services(), nil
	}

	return schema.GroupVersionKind{}, fmt.Errorf("backendRef group %q kind %q is not supported", group, kind)
}
