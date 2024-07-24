package cli

import (
	"context"
	"fmt"
	"github.com/loft-sh/log"
	"github.com/loft-sh/log/survey"
	"github.com/loft-sh/vcluster/pkg/cli/find"
	"github.com/loft-sh/vcluster/pkg/cli/flags"
	"github.com/loft-sh/vcluster/pkg/constants"
	"github.com/loft-sh/vcluster/pkg/lifecycle"
	"github.com/loft-sh/vcluster/pkg/platform"
	"github.com/loft-sh/vcluster/pkg/platform/clihelper"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"time"
)

type AddVClusterOptions struct {
	Project    string
	ImportName string
	Restart    bool
	Insecure   bool
	AccessKey  string
	Host       string
}

func AddVClusterHelm(
	ctx context.Context,
	options *AddVClusterOptions,
	globalFlags *flags.GlobalFlags,
	vClusterName string,
	log log.Logger,
) error {
	// check if vCluster exists
	vCluster, err := find.GetVCluster(ctx, globalFlags.Context, vClusterName, globalFlags.Namespace, log)
	if err != nil {
		return err
	}

	// create kube client
	restConfig, err := vCluster.ClientFactory.ClientConfig()
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	// If the vCluster was paused with the helm driver, adding it to the platform will only create the secret for registration
	// which leads to confusing behavior for the user since they won't see the cluster in the platform UI until it is resumed.
	if vCluster.Annotations != nil && vCluster.Annotations[constants.PausedAnnotation] == "true" {
		answer, err := log.Question(&survey.QuestionOptions{
			Question:     fmt.Sprintf("vCluster %s is asleep and needs to be reawakened before it can be added.  Would you like to wake it up and add it now?", vClusterName),
			DefaultValue: "no",
			Options: []string{
				"no",
				"yes",
			},
		})

		if err != nil {
			return fmt.Errorf("failed to capture your repsponse %w", err)
		}

		// Bail if they don't want to wake the vCluster yet.
		if answer == "no" {
			return fmt.Errorf("Please wakeup vCluster %s before adding it to the platform", vClusterName)
		}

		if err = ResumeHelm(ctx, globalFlags, vClusterName, log); err != nil {
			return fmt.Errorf("failed to wake up vCluster %s: %w", vClusterName, err)
		}

		err = wait.PollUntilContextTimeout(ctx, time.Second, clihelper.Timeout(), false, func(ctx context.Context) (done bool, err error) {
			vCluster, err = find.GetVCluster(ctx, globalFlags.Context, vClusterName, globalFlags.Namespace, log)
			if err != nil {
				return false, err
			}

			return vCluster.Annotations == nil || vCluster.Annotations[constants.PausedAnnotation] == "", nil
		})

		if err != nil {
			return fmt.Errorf("error waiting for vCluster to wake up %w", err)
		}

	}

	// apply platform secret
	err = platform.ApplyPlatformSecret(
		ctx,
		globalFlags.LoadedConfig(log),
		kubeClient,
		options.ImportName,
		vCluster.Namespace,
		options.Project,
		options.AccessKey,
		options.Host,
		options.Insecure,
	)
	if err != nil {
		return err
	}

	// restart vCluster
	if options.Restart {
		err = lifecycle.DeletePods(ctx, kubeClient, "app=vcluster,release="+vCluster.Name, vCluster.Namespace, log)
		if err != nil {
			return fmt.Errorf("delete vcluster workloads: %w", err)
		}
	}

	log.Donef("Successfully added vCluster %s/%s", vCluster.Namespace, vCluster.Name)
	return nil
}
