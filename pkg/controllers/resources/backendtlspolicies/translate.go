package backendtlspolicies

import (
	"fmt"

	"github.com/loft-sh/vcluster/pkg/controllers/resources/gatewayroutes"
	"github.com/loft-sh/vcluster/pkg/syncer/synccontext"
	"github.com/loft-sh/vcluster/pkg/util/translate"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (s *backendTLSPolicySyncer) translate(ctx *synccontext.SyncContext, vPolicy *gatewayv1.BackendTLSPolicy) (*gatewayv1.BackendTLSPolicy, error) {
	pPolicy := translate.HostMetadata(vPolicy, s.VirtualToHost(ctx, types.NamespacedName{Name: vPolicy.Name, Namespace: vPolicy.Namespace}, vPolicy))

	spec, err := translateSpecToHost(ctx, vPolicy, true)
	if err != nil {
		return nil, err
	}

	pPolicy.Spec = *spec
	return pPolicy, nil
}

func translateSpecToHost(ctx *synccontext.SyncContext, vPolicy *gatewayv1.BackendTLSPolicy, validateRefs bool) (*gatewayv1.BackendTLSPolicySpec, error) {
	retSpec := vPolicy.Spec.DeepCopy()

	for i := range retSpec.TargetRefs {
		err := translatePolicyTargetRefToHost(ctx, vPolicy.Namespace, &retSpec.TargetRefs[i], validateRefs)
		if err != nil {
			return nil, fmt.Errorf("translate targetRefs[%d]: %w", i, err)
		}
	}

	for i := range retSpec.Validation.CACertificateRefs {
		err := translateLocalObjectRefToHost(ctx, vPolicy.Namespace, &retSpec.Validation.CACertificateRefs[i], validateRefs)
		if err != nil {
			return nil, fmt.Errorf("translate validation.caCertificateRefs[%d]: %w", i, err)
		}
	}

	return retSpec, nil
}

func translatePolicyTargetRefToHost(ctx *synccontext.SyncContext, policyNamespace string, ref *gatewayv1.LocalPolicyTargetReferenceWithSectionName, validateRef bool) error {
	if validateRef {
		return gatewayroutes.TranslatePolicyTargetRefToHost(ctx, policyNamespace, ref)
	}

	return gatewayroutes.TranslatePolicyTargetRefToHostWithoutValidation(ctx, policyNamespace, ref)
}

func translateLocalObjectRefToHost(ctx *synccontext.SyncContext, policyNamespace string, ref *gatewayv1.LocalObjectReference, validateRef bool) error {
	if validateRef {
		return gatewayroutes.TranslateLocalObjectRefToHost(ctx, policyNamespace, ref)
	}

	return gatewayroutes.TranslateLocalObjectRefToHostWithoutValidation(ctx, policyNamespace, ref)
}
