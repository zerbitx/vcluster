package httproutes

import (
	"fmt"

	routetranslate "github.com/loft-sh/vcluster/pkg/controllers/resources/gatewayroutes/translate"
	"github.com/loft-sh/vcluster/pkg/syncer/synccontext"
	"github.com/loft-sh/vcluster/pkg/util/translate"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (s *httpRouteSyncer) translate(ctx *synccontext.SyncContext, vRoute *gatewayv1.HTTPRoute) (*gatewayv1.HTTPRoute, error) {
	pRoute := translate.HostMetadata(vRoute, s.VirtualToHost(ctx, types.NamespacedName{Name: vRoute.Name, Namespace: vRoute.Namespace}, vRoute))

	spec, err := translateSpecToHost(ctx, vRoute, true)
	if err != nil {
		return nil, err
	}

	pRoute.Spec = *spec
	return pRoute, nil
}

func translateSpecToHost(ctx *synccontext.SyncContext, vRoute *gatewayv1.HTTPRoute, validateRefs bool) (*gatewayv1.HTTPRouteSpec, error) {
	retSpec := vRoute.Spec.DeepCopy()

	for i := range retSpec.ParentRefs {
		err := translateParentRefToHost(ctx, vRoute.Namespace, &retSpec.ParentRefs[i], validateRefs)
		if err != nil {
			return nil, fmt.Errorf("translate parentRefs[%d]: %w", i, err)
		}
	}

	for i := range retSpec.Rules {
		err := translateRuleToHost(ctx, vRoute.Namespace, &retSpec.Rules[i], validateRefs)
		if err != nil {
			return nil, fmt.Errorf("translate rules[%d]: %w", i, err)
		}
	}

	return retSpec, nil
}

func translateStatusToVirtual(ctx *synccontext.SyncContext, hostRoute *gatewayv1.HTTPRoute, virtualRouteNamespace string, status gatewayv1.HTTPRouteStatus) (gatewayv1.HTTPRouteStatus, error) {
	retStatus := *status.DeepCopy()

	for i := range retStatus.Parents {
		hostRouteNamespace := routetranslate.ParentStatusHostNamespace(hostRoute.Namespace, hostRoute.Spec.ParentRefs, retStatus.Parents[i].ParentRef)
		err := routetranslate.ParentRefToVirtual(ctx, hostRouteNamespace, virtualRouteNamespace, &retStatus.Parents[i].ParentRef)
		if err != nil {
			return gatewayv1.HTTPRouteStatus{}, fmt.Errorf("translate parents[%d].parentRef: %w", i, err)
		}
	}

	return retStatus, nil
}

func translateRuleToHost(ctx *synccontext.SyncContext, routeNamespace string, rule *gatewayv1.HTTPRouteRule, validateRefs bool) error {
	for i := range rule.BackendRefs {
		err := translateHTTPBackendRefToHost(ctx, routeNamespace, &rule.BackendRefs[i], validateRefs)
		if err != nil {
			return fmt.Errorf("translate backendRefs[%d]: %w", i, err)
		}
	}

	for i := range rule.Filters {
		err := translateFilterToHost(ctx, routeNamespace, &rule.Filters[i], validateRefs)
		if err != nil {
			return fmt.Errorf("translate filters[%d]: %w", i, err)
		}
	}

	return nil
}

func translateHTTPBackendRefToHost(ctx *synccontext.SyncContext, routeNamespace string, ref *gatewayv1.HTTPBackendRef, validateRef bool) error {
	err := translateBackendObjectRefToHost(ctx, routeNamespace, &ref.BackendObjectReference, validateRef)
	if err != nil {
		return err
	}

	for i := range ref.Filters {
		err := translateFilterToHost(ctx, routeNamespace, &ref.Filters[i], validateRef)
		if err != nil {
			return fmt.Errorf("translate filters[%d]: %w", i, err)
		}
	}

	return nil
}

func translateParentRefToHost(ctx *synccontext.SyncContext, routeNamespace string, ref *gatewayv1.ParentReference, validateRef bool) error {
	if validateRef {
		return routetranslate.ParentRefToHost(ctx, routeNamespace, ref)
	}

	return routetranslate.ParentRefToHostWithoutValidation(ctx, routeNamespace, ref)
}

func translateBackendObjectRefToHost(ctx *synccontext.SyncContext, routeNamespace string, ref *gatewayv1.BackendObjectReference, validateRef bool) error {
	if validateRef {
		return routetranslate.BackendObjectRefToHost(ctx, routeNamespace, ref)
	}

	return routetranslate.BackendObjectRefToHostWithoutValidation(ctx, routeNamespace, ref)
}

func translateFilterToHost(ctx *synccontext.SyncContext, routeNamespace string, filter *gatewayv1.HTTPRouteFilter, validateRefs bool) error {
	if filter.RequestMirror != nil {
		err := translateBackendObjectRefToHost(ctx, routeNamespace, &filter.RequestMirror.BackendRef, validateRefs)
		if err != nil {
			return fmt.Errorf("translate requestMirror.backendRef: %w", err)
		}
	}

	if filter.ExternalAuth != nil {
		err := translateBackendObjectRefToHost(ctx, routeNamespace, &filter.ExternalAuth.BackendRef, validateRefs)
		if err != nil {
			return fmt.Errorf("translate externalAuth.backendRef: %w", err)
		}
	}

	return nil
}
