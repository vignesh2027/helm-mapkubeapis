package v3

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func TestV3(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Release mapping suite")
}

func newConfig() *action.Configuration {
	return &action.Configuration{
		Releases: storage.Init(driver.NewMemory()),
		Log:      func(string, ...interface{}) {},
	}
}

func storeRelease(cfg *action.Configuration, version int, status release.Status, manifest string) {
	rel := &release.Release{
		Name:      "demo",
		Namespace: "default",
		Version:   version,
		Manifest:  manifest,
		Chart:     &chart.Chart{Metadata: &chart.Metadata{Name: "demo", Version: "0.1.0"}},
		Info:      &release.Info{Status: status},
	}
	gomega.Expect(cfg.Releases.Create(rel)).To(gomega.Succeed())
}

var _ = ginkgo.Describe("release selection", func() {
	var cfg *action.Configuration

	ginkgo.BeforeEach(func() {
		cfg = newConfig()
	})

	ginkgo.It("maps the latest version when it is deployed", func() {
		storeRelease(cfg, 1, release.StatusSuperseded, "one")
		storeRelease(cfg, 2, release.StatusDeployed, "two")

		toMap, latest, err := getReleaseToMap("demo", cfg)

		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(toMap.Version).To(gomega.Equal(2))
		gomega.Expect(latest.Version).To(gomega.Equal(2))
	})

	ginkgo.It("maps the deployed version when later versions failed", func() {
		storeRelease(cfg, 1, release.StatusDeployed, "one")
		storeRelease(cfg, 2, release.StatusFailed, "two")
		storeRelease(cfg, 3, release.StatusFailed, "three")

		toMap, latest, err := getReleaseToMap("demo", cfg)

		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(toMap.Version).To(gomega.Equal(1))
		gomega.Expect(latest.Version).To(gomega.Equal(3))
	})

	ginkgo.It("falls back to the latest version when nothing is deployed", func() {
		storeRelease(cfg, 1, release.StatusFailed, "one")
		storeRelease(cfg, 2, release.StatusFailed, "two")

		toMap, latest, err := getReleaseToMap("demo", cfg)

		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(toMap.Version).To(gomega.Equal(2))
		gomega.Expect(latest.Version).To(gomega.Equal(2))
	})

	ginkgo.It("reports a release that does not exist", func() {
		_, _, err := getReleaseToMap("missing", cfg)

		gomega.Expect(err).To(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("release update", func() {
	var cfg *action.Configuration

	ginkgo.BeforeEach(func() {
		cfg = newConfig()
	})

	ginkgo.It("numbers the new version after the latest one", func() {
		storeRelease(cfg, 1, release.StatusDeployed, "old")
		storeRelease(cfg, 2, release.StatusFailed, "failed")
		toMap, latest, err := getReleaseToMap("demo", cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Expect(updateRelease(toMap, latest, "new", cfg)).To(gomega.Succeed())

		current, err := cfg.Releases.Deployed("demo")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(current.Version).To(gomega.Equal(3))
		gomega.Expect(current.Manifest).To(gomega.Equal("new"))
		gomega.Expect(current.Info.Description).To(gomega.Equal("Kubernetes deprecated API upgrade - DO NOT rollback from this version"))

		first, err := cfg.Releases.Get("demo", 1)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(first.Info.Status).To(gomega.Equal(release.StatusSuperseded))
		gomega.Expect(first.Manifest).To(gomega.Equal("old"))

		failed, err := cfg.Releases.Get("demo", 2)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(failed.Info.Status).To(gomega.Equal(release.StatusFailed))
	})

	ginkgo.It("adds the next version when the latest one is deployed", func() {
		storeRelease(cfg, 1, release.StatusDeployed, "old")
		toMap, latest, err := getReleaseToMap("demo", cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Expect(updateRelease(toMap, latest, "new", cfg)).To(gomega.Succeed())

		current, err := cfg.Releases.Last("demo")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(current.Version).To(gomega.Equal(2))
		gomega.Expect(current.Info.Status).To(gomega.Equal(release.StatusDeployed))
		gomega.Expect(current.Manifest).To(gomega.Equal("new"))

		first, err := cfg.Releases.Get("demo", 1)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(first.Info.Status).To(gomega.Equal(release.StatusSuperseded))
		gomega.Expect(first.Manifest).To(gomega.Equal("old"))
	})
})
