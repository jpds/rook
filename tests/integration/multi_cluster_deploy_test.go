package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestInstallingMultipleRookCluster(t *testing.T) {
	suite.Run(t, new(MultiRookClusterDeploySuite))
}

type MultiRookClusterDeploySuite struct {
	suite.Suite
	helper1     *clients.TestClient
	helper2     *clients.TestClient
	k8sh        *utils.K8sHelper
	installer   *installer.InstallHelper
	installData *installer.InstallData
}

//Deploy Multiple Rook clusters
func (mrc *MultiRookClusterDeploySuite) SetupSuite() {
	//Skipping test until https://github.com/rook/rook/issues/970 is fixed.
	mrc.Suite.T().SkipNow()
	kh, err := utils.CreateK8sHelper(mrc.T)
	require.NoError(mrc.T(), err)

	mrc.k8sh = kh
	mrc.installer = installer.NewK8sRookhelper(kh.Clientset, mrc.T)
	mrc.installData = installer.NewK8sInstallData()

	err = mrc.installer.CreateK8sRookOperator()
	require.NoError(mrc.T(), err)

	require.True(mrc.T(), kh.IsPodInExpectedState("rook-operator", defaultRookSystemNamespace, "Running"),
		"Make sure rook-operator is in running state")

	require.True(mrc.T(), kh.IsPodInExpectedState("rook-agent", defaultRookSystemNamespace, "Running"),
		"Make sure rook-agent is in running state")

	time.Sleep(10 * time.Second)

	err = mrc.installer.CreateK8sRookCluster(clusterNamespace1)
	require.NoError(mrc.T(), err)

	err = mrc.installer.CreateK8sRookToolbox(clusterNamespace1)
	require.NoError(mrc.T(), err)

	err = mrc.installer.CreateK8sRookCluster(clusterNamespace2)
	require.NoError(mrc.T(), err)

	err = mrc.installer.CreateK8sRookToolbox(clusterNamespace2)
	require.NoError(mrc.T(), err)

	mrc.helper1, err = clients.CreateTestClient(enums.Kubernetes, kh, clusterNamespace1)
	require.Nil(mrc.T(), err)

	mrc.helper2, err = clients.CreateTestClient(enums.Kubernetes, kh, clusterNamespace2)
	require.Nil(mrc.T(), err)

}

func (mrc *MultiRookClusterDeploySuite) TearDownSuite() {
	if mrc.T().Failed() {
		gatherAllRookLogs(mrc.k8sh, mrc.Suite, defaultRookSystemNamespace, clusterNamespace1)
		gatherAllRookLogs(mrc.k8sh, mrc.Suite, defaultRookSystemNamespace, clusterNamespace2)
	}
	deleteArgs := []string{"delete", "-f", "-"}

	skipRookInstall := strings.EqualFold(mrc.installer.Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}

	logger.Infof("Uninstalling All Rook Clusters")
	k8sHelp, err := utils.CreateK8sHelper(mrc.T)
	if err != nil {
		panic(err)
	}
	rookOperator := mrc.installData.GetRookOperator(mrc.k8sh.GetK8sServerVersion())

	//Delete rook operator
	_, err = mrc.k8sh.KubectlWithStdin(rookOperator, deleteArgs...)
	if err != nil {
		logger.Errorf("Rook operator cannot be deleted,err -> %v", err)
		panic(err)
	}

	//delete rook cluster
	mrc.installer.CleanupCluster(clusterNamespace1)
	mrc.installer.CleanupCluster(clusterNamespace2)

	//Delete ClusterRole and ClusterRoleBindings
	if mrc.k8sh.VersionAtLeast("v1.6.0") {
		err = k8sHelp.DeleteClusterRoleAndBindings("rook-api")
		if err != nil {
			logger.Errorf("rook-api cluster role and binding cannot be deleted: %+v", err)
			panic(err)
		}
		err = k8sHelp.DeleteClusterRoleAndBindings("rook-ceph-osd")
		if err != nil {
			logger.Errorf("rook-ceph-osd cluster role and binding cannot be deleted: %+v", err)
			panic(err)
		}
	}

	// Delete crd/tpr
	if mrc.k8sh.VersionAtLeast("v1.7.0") {
		_, err = k8sHelp.DeleteResource([]string{"crd", "clusters.rook.io", "pools.rook.io", "objectstores.rook.io"})
		if err != nil {
			logger.Errorf("crd cannot be deleted")
			panic(err)
		}
	} else {
		_, err = k8sHelp.DeleteResource([]string{"thirdpartyresources", "cluster.rook.io", "pool.rook.io", "objectstore.rook.io"})
		if err != nil {
			logger.Errorf("tpr cannot be deleted")
			panic(err)
		}
	}

	isRookUninstalled1 := k8sHelp.WaitUntilPodInNamespaceIsDeleted("rook-ceph-mon", clusterNamespace1)
	isNameSpaceDeleted1 := k8sHelp.WaitUntilNameSpaceIsDeleted(clusterNamespace1)
	isRookUninstalled2 := k8sHelp.WaitUntilPodInNamespaceIsDeleted("rook-ceph-mon", clusterNamespace2)
	isNameSpaceDeleted2 := k8sHelp.WaitUntilNameSpaceIsDeleted(clusterNamespace2)
	mrc.k8sh.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)

	if isRookUninstalled1 && isNameSpaceDeleted1 && isRookUninstalled2 && isNameSpaceDeleted2 {
		logger.Infof("Rook clusters %s  and  %s uninstalled successfully", clusterNamespace1, clusterNamespace2)
		return
	}
	logger.Infof("Rook clusters %s  and  %s  not uninstalled successfully", clusterNamespace1, clusterNamespace2)

}

//Test to make sure all rook components are installed and Running
func (mrc *MultiRookClusterDeploySuite) TestInstallingMultipleRookClusters() {

	//Check if Rook cluster 1 is deployed successfully
	checkIfRookClusterIsInstalled(mrc.k8sh, mrc.Suite, defaultRookSystemNamespace, clusterNamespace1)

	//Check if Rook cluster 1 is deployed successfully
	checkIfRookClusterIsInstalled(mrc.k8sh, mrc.Suite, defaultRookSystemNamespace, clusterNamespace2)

}

//Test Block Store Creation on multiple rook clusters
func (mrc *MultiRookClusterDeploySuite) TestBlockStoreOnMultipleRookCluster() {
	runBlockE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, clusterNamespace1)
	runBlockE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, clusterNamespace2)

}

//Test Filesystem Creation on multiple rook clusters
func (mrc *MultiRookClusterDeploySuite) TestFileStoreOnMultiRookCluster() {
	runFileE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, clusterNamespace1, "test-fs-1")
	//TODO - Known Issues #https://github.com/rook/rook/issues/970
	//runFileE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, clusterNamespace2, "test-fs-2")

}

//Test Object Store Creation on multiple rook clusters
func (mrc *MultiRookClusterDeploySuite) TestObjectStoreOnMultiRookCluster() {
	runObjectE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, clusterNamespace1, "default-c1", 2)
	//TODO - Known Issues #https://github.com/rook/rook/issues/970
	//runObjectE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, clusterNamespace2, "default-c2", 1)

}
