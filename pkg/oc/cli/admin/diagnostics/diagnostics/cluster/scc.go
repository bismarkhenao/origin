package cluster

import (
	"fmt"
	"io/ioutil"

	kerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/apis/authorization"
	authorizationtypedclient "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset/typed/authorization/internalversion"

	"github.com/openshift/origin/pkg/cmd/server/bootstrappolicy"
	"github.com/openshift/origin/pkg/oc/cli/admin/diagnostics/diagnostics/types"
	"github.com/openshift/origin/pkg/oc/cli/admin/diagnostics/diagnostics/util"
	policycmd "github.com/openshift/origin/pkg/oc/cli/admin/policy"
	securityapi "github.com/openshift/origin/pkg/security/apis/security"
	securitytypedclient "github.com/openshift/origin/pkg/security/generated/internalclientset/typed/security/internalversion"
)

// SCC is a Diagnostic to check that the default securitycontextconstraints are present and contain the expected permissions.
type SCC struct {
	SCCClient securitytypedclient.SecurityContextConstraintsInterface
	SARClient authorizationtypedclient.SelfSubjectAccessReviewsGetter
}

const SccName = "SecurityContextConstraints"

func (d *SCC) Name() string {
	return SccName
}

func (d *SCC) Description() string {
	return "Check that the default SecurityContextConstraints are present and contain the expected permissions"
}

func (d *SCC) Requirements() (client bool, host bool) {
	return true, false
}

func (d *SCC) CanRun() (bool, error) {
	if d.SARClient == nil {
		return false, fmt.Errorf("must have client.SubjectAccessReviews")
	}

	return util.UserCan(d.SARClient, &authorization.ResourceAttributes{
		Verb:     "list",
		Group:    securityapi.GroupName,
		Resource: "securitycontextconstraints",
	})
}

func (d *SCC) Check() types.DiagnosticResult {
	r := types.NewDiagnosticResult(SccName)
	reconcileOptions := &policycmd.ReconcileSCCOptions{
		Confirmed:      false,
		Union:          true,
		Out:            ioutil.Discard,
		SCCClient:      d.SCCClient,
		InfraNamespace: bootstrappolicy.DefaultOpenShiftInfraNamespace,
	}

	changedSCCs, err := reconcileOptions.ChangedSCCs()
	if err != nil {
		r.Error("CSD1000", err, fmt.Sprintf("Error inspecting SCCs: %v", err))
		return r
	}
	changedSCCNames := map[string]bool{}
	for _, changedSCC := range changedSCCs {
		_, err := d.SCCClient.Get(changedSCC.Name, metav1.GetOptions{})
		if kerrs.IsNotFound(err) {
			r.Error("CSD1001", nil, fmt.Sprintf("scc/%s is missing.\n\nUse the `oc adm policy reconcile-sccs` command to recreate sccs.", changedSCC.Name))
			continue
		}
		if err != nil {
			r.Error("CSD1002", err, fmt.Sprintf("Unable to get scc/%s: %v", changedSCC.Name, err))
			continue
		}
		r.Warn("CSD1003", nil, fmt.Sprintf("scc/%s will be reconciled. Use the `oc adm policy reconcile-sccs` command to check sccs.", changedSCC.Name))
		changedSCCNames[changedSCC.Name] = true
	}

	// Including non-additive SCCs, but output messages with debug level.
	reconcileOptions.Union = false
	changedSCCs, err = reconcileOptions.ChangedSCCs()
	if err != nil {
		r.Error("CSD1000", err, fmt.Sprintf("Error inspecting SCCs: %v", err))
		return r
	}
	for _, changedSCC := range changedSCCs {
		if !changedSCCNames[changedSCC.Name] {
			r.Debug("CSD1004", fmt.Sprintf("scc/%s does not match defaults. Use the `oc adm policy reconcile-sccs --additive-only=false` command to check sccs.", changedSCC.Name))
		}
	}
	return r
}
