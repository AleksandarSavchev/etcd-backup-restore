package member_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gardener/etcd-backup-restore/pkg/member"
	mockfactory "github.com/gardener/etcd-backup-restore/pkg/mock/etcdutil/client"
	brtypes "github.com/gardener/etcd-backup-restore/pkg/types"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/etcdserver/etcdserverpb"
)

var _ = Describe("Membercontrol", func() {
	var (
		etcdConnectionConfig *brtypes.EtcdConnectionConfig
		ctrl                 *gomock.Controller
		factory              *mockfactory.MockFactory
		cl                   *mockfactory.MockClusterCloser
	)
	const (
		podName = "test-pod"
	)

	BeforeEach(func() {
		etcdConnectionConfig = brtypes.NewEtcdConnectionConfig()
		etcdConnectionConfig.Endpoints = []string{etcd.Clients[0].Addr().String()}
		etcdConnectionConfig.ConnectionTimeout.Duration = 30 * time.Second
		etcdConnectionConfig.SnapshotTimeout.Duration = 30 * time.Second
		etcdConnectionConfig.DefragTimeout.Duration = 30 * time.Second

		os.Setenv("POD_NAME", podName)

		ctrl = gomock.NewController(GinkgoT())
		factory = mockfactory.NewMockFactory(ctrl)
		cl = mockfactory.NewMockClusterCloser(ctrl)

		outfile := "/tmp/etcd.conf.yaml"
		etcdConfigYaml := `# Human-readable name for this member.
    name: etcd1
    data-dir: ` + os.Getenv("ETCD_DATA_DIR") + `
    metrics: extensive
    snapshot-count: 75000
    enable-v2: false
    quota-backend-bytes: 1073741824
    listen-client-urls: http://0.0.0.0:2379
    advertise-client-urls: http://0.0.0.0:2379
    initial-advertise-peer-urls: http@etcd-main-peer@default@2380
    initial-cluster: etcd1=http://0.0.0.0:2380
    initial-cluster-token: new
    initial-cluster-state: new
    auto-compaction-mode: periodic
    auto-compaction-retention: 30m`

		err := os.WriteFile(outfile, []byte(etcdConfigYaml), 0755)
		Expect(err).ShouldNot(HaveOccurred())
		os.Setenv("ETCD_CONF", outfile)

	})

	AfterEach(func() {
		os.Unsetenv("POD_NAME")
		os.Unsetenv("ETCD_CONF")
	})

	Describe("Creating NewMemberControl", func() {
		Context("With valid configuration", func() {
			It("should return memberControl", func() {
				ctrlMember := member.NewMemberControl(etcdConnectionConfig)
				Expect(ctrlMember).ShouldNot(BeNil())
			})
		})
	})

	Describe("While attempting to add a new member as a learner", func() {
		Context("Member is not already part of the cluster", func() {
			It("Should add member to the cluster as a learner", func() {
				mem := member.NewMemberControl(etcdConnectionConfig)
				err := mem.AddMemberAsLearner(context.TODO())
				Expect(err).To(BeNil())
				present, err := mem.IsLearnerPresent(context.TODO())
				Expect(err).To(BeNil())
				Expect(present).To(BeTrue())
			})
		})
	})

	Describe("While attempting to check if etcd is part of a cluster", func() {
		Context("When cluster is up and member is not part of the list", func() {
			It("Should return false and no error", func() {
				mem := member.NewMemberControl(etcdConnectionConfig)
				bool, err := mem.IsMemberInCluster(context.TODO())
				Expect(bool).To(BeFalse())
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("Update Etcd cluster member peer address", func() {
		var (
			dummyID = uint64(1111)
			m       member.Control
		)
		BeforeEach(func() {
			factory.EXPECT().NewCluster().Return(cl, nil)
			m = member.NewMemberControl(etcdConnectionConfig)
		})

		Context("Able to connect to etcd member", func() {
			It("Should not return error", func() {
				client, err := factory.NewCluster()
				Expect(err).ShouldNot(HaveOccurred())

				cl.EXPECT().MemberList(gomock.Any()).DoAndReturn(func(_ context.Context) (*clientv3.MemberListResponse, error) {
					etcdMember1 := &etcdserverpb.Member{
						ID: dummyID,
					}
					etcdMember2 := &etcdserverpb.Member{
						ID: dummyID + 1,
					}
					response := new(clientv3.MemberListResponse)

					response.Members = append(response.Members, etcdMember1, etcdMember2)
					response.Members = []*etcdserverpb.Member{etcdMember1, etcdMember2}
					response.Header = &etcdserverpb.ResponseHeader{
						MemberId: dummyID,
					}
					return response, nil
				})

				cl.EXPECT().MemberUpdate(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

				err = m.UpdateMemberPeerURL(context.TODO(), client)
				Expect(err).ShouldNot(HaveOccurred())
			})
		})

		Context("Unable to connect to etcd member for MemberUpdate api call", func() {
			It("Should return error", func() {
				client, err := factory.NewCluster()
				Expect(err).ShouldNot(HaveOccurred())

				cl.EXPECT().MemberList(gomock.Any()).DoAndReturn(func(_ context.Context) (*clientv3.MemberListResponse, error) {
					etcdMember1 := &etcdserverpb.Member{
						ID: dummyID,
					}
					etcdMember2 := &etcdserverpb.Member{
						ID: dummyID + 1,
					}
					response := new(clientv3.MemberListResponse)

					response.Members = append(response.Members, etcdMember1, etcdMember2)
					response.Members = []*etcdserverpb.Member{etcdMember1, etcdMember2}
					response.Header = &etcdserverpb.ResponseHeader{
						MemberId: dummyID,
					}
					return response, nil
				})

				cl.EXPECT().MemberUpdate(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("unable to connect to dummy etcd"))

				err = m.UpdateMemberPeerURL(context.TODO(), client)
				Expect(err).Should(HaveOccurred())
			})
		})
	})
})
