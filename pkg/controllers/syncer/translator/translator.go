package translator

import (
	"context"

	syncercontext "github.com/loft-sh/vcluster/pkg/controllers/syncer/context"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Translator is used to translate names as well as metadata between virtual and physical objects
type Translator interface {
	Resource() client.Object
	Name() string
	ObjectManager
	MetadataTranslator
}

// ObjectManager is used to convert virtual to physical names and vice versa
type ObjectManager interface {
	// IsManaged determines if a physical object is managed by the vcluster
	IsManaged(context.Context, client.Object) (bool, error)
}

// MetadataTranslator is used to convert metadata between virtual and physical objects and vice versa
type MetadataTranslator interface {
	// TranslateMetadata translates the object's metadata
	TranslateMetadata(ctx context.Context, vObj client.Object) client.Object

	// TranslateMetadataUpdate translates the object's metadata annotations and labels and determines
	// if they have changed between the physical and virtual object
	TranslateMetadataUpdate(ctx context.Context, vObj client.Object, pObj client.Object) (changed bool, annotations map[string]string, labels map[string]string)
}

// NamespacedTranslator provides some helper functions to ease sync down translation
type NamespacedTranslator interface {
	Translator

	// EventRecorder returns
	EventRecorder() record.EventRecorder

	// SyncToHostCreate creates the given pObj in the target namespace
	SyncToHostCreate(ctx *syncercontext.SyncContext, vObj, pObj client.Object) (ctrl.Result, error)

	// SyncToHostUpdate updates the given pObj (if not nil) in the target namespace
	SyncToHostUpdate(ctx *syncercontext.SyncContext, vObj, pObj client.Object) (ctrl.Result, error)
}
