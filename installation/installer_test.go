package installation_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/cloudfoundry/bosh-micro-cli/installation"

	"code.google.com/p/gomock/gomock"
	mock_install_job "github.com/cloudfoundry/bosh-micro-cli/installation/job/mocks"
	mock_install_pkg "github.com/cloudfoundry/bosh-micro-cli/installation/pkg/mocks"
	mock_install_state "github.com/cloudfoundry/bosh-micro-cli/installation/state/mocks"
	mock_registry "github.com/cloudfoundry/bosh-micro-cli/registry/mocks"

	boshlog "github.com/cloudfoundry/bosh-agent/logger"

	bminstalljob "github.com/cloudfoundry/bosh-micro-cli/installation/job"
	bminstallmanifest "github.com/cloudfoundry/bosh-micro-cli/installation/manifest"
	bminstallpkg "github.com/cloudfoundry/bosh-micro-cli/installation/pkg"
	bminstallstate "github.com/cloudfoundry/bosh-micro-cli/installation/state"
	bmrel "github.com/cloudfoundry/bosh-micro-cli/release"
	bmreljob "github.com/cloudfoundry/bosh-micro-cli/release/job"
	bmrelpkg "github.com/cloudfoundry/bosh-micro-cli/release/pkg"

	fakesys "github.com/cloudfoundry/bosh-agent/system/fakes"
	fakebmeventlog "github.com/cloudfoundry/bosh-micro-cli/eventlogger/fakes"
	testfakes "github.com/cloudfoundry/bosh-micro-cli/testutils/fakes"
	fakebmui "github.com/cloudfoundry/bosh-micro-cli/ui/fakes"
)

var _ = Describe("Installer", func() {
	var mockCtrl *gomock.Controller

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	var (
		fakeFS        *fakesys.FakeFileSystem
		fakeExtractor *testfakes.FakeMultiResponseExtractor
		fakeUI        *fakebmui.FakeUI

		mockStateBuilder     *mock_install_state.MockBuilder
		mockPackageInstaller *mock_install_pkg.MockPackageInstaller
		mockJobInstaller     *mock_install_job.MockInstaller

		mockRegistryServerManager *mock_registry.MockServerManager

		logger boshlog.Logger

		packagesPath           string
		deploymentManifestPath string
		installer              Installer
		target                 Target
	)

	BeforeEach(func() {
		fakeFS = fakesys.NewFakeFileSystem()
		fakeExtractor = testfakes.NewFakeMultiResponseExtractor()
		fakeUI = &fakebmui.FakeUI{}

		logger = boshlog.NewLogger(boshlog.LevelNone)

		mockStateBuilder = mock_install_state.NewMockBuilder(mockCtrl)
		mockPackageInstaller = mock_install_pkg.NewMockPackageInstaller(mockCtrl)
		mockJobInstaller = mock_install_job.NewMockInstaller(mockCtrl)

		mockRegistryServerManager = mock_registry.NewMockServerManager(mockCtrl)

		packagesPath = "/path/to/installed/packages"
		deploymentManifestPath = "/path/to/manifest.yml"
		target = NewTarget("fake-installation-path")
	})

	JustBeforeEach(func() {
		installer = NewInstaller(
			target,
			fakeFS,
			mockStateBuilder,
			packagesPath,
			mockPackageInstaller,
			mockJobInstaller,
			mockRegistryServerManager,
			logger,
		)
	})

	Describe("Install", func() {
		var (
			installationManifest bminstallmanifest.Manifest
			release              bmrel.Release
			releaseJob           bmreljob.Job
			fakeStage            *fakebmeventlog.FakeStage

			installedJob bminstalljob.InstalledJob

			expectStateBuild     *gomock.Call
			expectPackageInstall *gomock.Call
			expectJobInstall     *gomock.Call
		)

		BeforeEach(func() {
			fakeFS.WriteFileString(deploymentManifestPath, "")

			installationManifest = bminstallmanifest.Manifest{
				Name:          "fake-installation-name",
				Release:       "fake-release-name",
				RawProperties: map[interface{}]interface{}{},
			}

			fakeStage = fakebmeventlog.NewFakeStage()

			releaseJob = bmreljob.Job{Name: "cpi"}

			installedJob = bminstalljob.InstalledJob{
				Name: "cpi",
				Path: "/extracted-release-path/cpi",
			}
		})

		JustBeforeEach(func() {
			releaseJobs := []bmreljob.Job{releaseJob}
			releasePackages := append([]*bmrelpkg.Package(nil), releaseJob.Packages...)
			release = bmrel.NewRelease(
				"fake-release-name",
				"fake-release-version",
				releaseJobs,
				releasePackages,
				"/extracted-release-path",
				fakeFS,
			)

			renderedCPIJob := bminstalljob.RenderedJobRef{
				Name:        "cpi",
				Version:     "fake-release-job-fingerprint",
				BlobstoreID: "fake-rendered-job-blobstore-id",
				SHA1:        "fake-rendered-job-blobstore-id",
			}

			compiledPackageRef := bminstallpkg.CompiledPackageRef{
				Name:        "fake-release-package-name",
				Version:     "fake-release-package-fingerprint",
				BlobstoreID: "fake-compiled-package-blobstore-id",
				SHA1:        "fake-compiled-package-blobstore-id",
			}
			compiledPackages := []bminstallpkg.CompiledPackageRef{compiledPackageRef}

			state := bminstallstate.NewState(renderedCPIJob, compiledPackages)

			expectStateBuild = mockStateBuilder.EXPECT().Build(installationManifest, fakeStage).Return(state, nil).AnyTimes()

			expectPackageInstall = mockPackageInstaller.EXPECT().Install(compiledPackageRef, packagesPath).AnyTimes()

			expectJobInstall = mockJobInstaller.EXPECT().Install(renderedCPIJob, fakeStage).Return(installedJob, nil).AnyTimes()

			fakeFS.MkdirAll("/extracted-release-path", os.FileMode(0750))
		})

		It("builds a new installation state", func() {
			expectStateBuild.Times(1)

			_, err := installer.Install(installationManifest, fakeStage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("installs the compiled packages specified by the state", func() {
			expectPackageInstall.Times(1)

			_, err := installer.Install(installationManifest, fakeStage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("installs the rendered jobs specified by the state", func() {
			expectJobInstall.Times(1)

			_, err := installer.Install(installationManifest, fakeStage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the installation", func() {
			installation, err := installer.Install(installationManifest, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			expectedInstallation := NewInstallation(
				target,
				installedJob,
				installationManifest,
				mockRegistryServerManager,
			)

			Expect(installation).To(Equal(expectedInstallation))
		})
	})
})
