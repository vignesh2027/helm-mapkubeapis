/*
Copyright

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v3

import (
	"fmt"
	"log"

	"github.com/pkg/errors"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"

	common "github.com/helm/helm-mapkubeapis/pkg/common"
	"github.com/helm/helm-mapkubeapis/pkg/mapping"
)

// MapReleaseWithUnSupportedAPIs checks the release version that Helm would upgrade from for any deprecated or removed
// APIs in its metadata. That is the last deployed version, or the latest version when nothing is deployed. If it finds
// any, it will create a new release version with the APIs mapped to the supported versions
func MapReleaseWithUnSupportedAPIs(mapOptions common.MapOptions, additionalMappings ...*mapping.Mapping) error {
	cfg, err := GetActionConfig(mapOptions.ReleaseNamespace, mapOptions.KubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to get Helm action configuration")
	}

	var releaseName = mapOptions.ReleaseName
	log.Printf("Get release '%s' latest version.\n", releaseName)
	releaseToMap, latestRelease, err := getReleaseToMap(releaseName, cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to get release '%s' latest version", mapOptions.ReleaseName)
	}

	log.Printf("Check release '%s' for deprecated or removed APIs...\n", releaseName)
	var origManifest = releaseToMap.Manifest
	modifiedManifest, err := common.ReplaceManifestUnSupportedAPIs(origManifest, mapOptions.MapFile, mapOptions.KubeConfig, additionalMappings...)
	if err != nil {
		return err
	}
	log.Printf("Finished checking release '%s' for deprecated or removed APIs.\n", releaseName)
	if modifiedManifest == origManifest {
		log.Printf("Release '%s' has no deprecated or removed APIs.\n", releaseName)
		return nil
	}

	if mapOptions.DryRun {
		log.Printf("Deprecated or removed APIs exist, for release: %s.\n", releaseName)
	} else {
		log.Printf("Deprecated or removed APIs exist, updating release: %s.\n", releaseName)
		if err := updateRelease(releaseToMap, latestRelease, modifiedManifest, cfg); err != nil {
			return errors.Wrapf(err, "failed to update release '%s'", releaseName)
		}
		log.Printf("Release '%s' with deprecated or removed APIs updated successfully to new version.\n", releaseName)
	}

	return nil
}

// updateRelease supersedes origRelease and stores a copy of it carrying the modified manifest. The copy is numbered
// after latestRelease so that it does not collide with newer versions that are not deployed, such as failed ones.
func updateRelease(origRelease, latestRelease *release.Release, modifiedManifest string, cfg *action.Configuration) error {
	// Update current release version to be superseded
	log.Printf("Set status of release version '%s' to 'superseded'.\n", getReleaseVersionName(origRelease))
	origRelease.Info.Status = release.StatusSuperseded
	if err := cfg.Releases.Update(origRelease); err != nil {
		return errors.Wrapf(err, "failed to update release version '%s'", getReleaseVersionName(origRelease))
	}
	log.Printf("Release version '%s' updated successfully.\n", getReleaseVersionName(origRelease))

	// Using a shallow copy of current release version to update the object with the modification
	// and then store this new version
	var newRelease = *origRelease
	info := *origRelease.Info
	newRelease.Info = &info
	newRelease.Manifest = modifiedManifest
	newRelease.Info.Description = common.UpgradeDescription
	newRelease.Info.LastDeployed = cfg.Now()
	newRelease.Version = latestRelease.Version + 1
	newRelease.Info.Status = release.StatusDeployed
	log.Printf("Add release version '%s' with updated supported APIs.\n", getReleaseVersionName(&newRelease))
	if err := cfg.Releases.Create(&newRelease); err != nil {
		return errors.Wrapf(err, "failed to create new release version '%s'", getReleaseVersionName(&newRelease))
	}
	log.Printf("Release version '%s' added successfully.\n", getReleaseVersionName(&newRelease))
	return nil
}

// getReleaseToMap returns the release version whose manifest has to be mapped, together with the latest version of
// the release. Like helm upgrade, it picks the last deployed version and only falls back to the latest version when
// nothing is deployed, so that a trailing failed revision does not hide the manifest Helm will upgrade from.
func getReleaseToMap(releaseName string, cfg *action.Configuration) (*release.Release, *release.Release, error) {
	latest, err := cfg.Releases.Last(releaseName)
	if err != nil {
		return nil, nil, err
	}
	if latest.Info.Status == release.StatusDeployed {
		return latest, latest, nil
	}

	deployed, err := cfg.Releases.Deployed(releaseName)
	if err != nil {
		if errors.Is(err, driver.ErrNoDeployedReleases) {
			return latest, latest, nil
		}
		return nil, nil, err
	}
	return deployed, latest, nil
}

func getReleaseVersionName(rel *release.Release) string {
	return fmt.Sprintf("%s.v%d", rel.Name, rel.Version)
}
