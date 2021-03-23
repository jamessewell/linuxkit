package main

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/diskfs/go-diskfs"
	log "github.com/sirupsen/logrus"
)

const (
	configDriveMetadataFile    = "meta_data.json"
	configDriveUserdataFile    = "user_data"
	configDriveNetworkdataFile = "network_data.json"
	configDriveCdromDevs       = "/dev/sr[0-9]*"
	configDriveBlockDevs       = "/sys/class/block/*"
)

// ProviderConfigDrive is the type implementing the Provider interface for ConfigDrives
// It looks for file called 'meta-data', 'user-data' or 'config' in the root
type ProviderConfigDrive struct {
	device             string
	mountPoint         string
	err                error
	userdata, metadata []byte
}

// ListConfigDrives lists all the cdroms in the system
func ListConfigDrives() []Provider {
	// get the devices that match the cloud-init spec
	configdrives := FindConfigDrives()
	log.Debugf("config-2 devices to be checked: %v", configdrives)
	providers := []Provider{}
	for _, device := range configdrives {
		providers = append(providers, NewConfigDrive(device))
	}
	return providers
}

// FindConfigDrives goes through all known devices. Returns any that are either fat32 or
// iso9660 and have a filesystem label "config-2" per the spec
// here https://github.com/canonical/cloud-init/blob/master/doc/rtd/topics/datasources/nocloud.rst
func FindConfigDrives() []string {
	devs, err := filepath.Glob(configDriveBlockDevs)
	log.Debugf("block devices found: %v", devs)
	if err != nil {
		// Glob can only error on invalid pattern
		panic(fmt.Sprintf("Invalid glob pattern: %s", configDriveBlockDevs))
	}
	foundDevices := []string{}
	for _, device := range devs {
		// get the base device name
		dev := filepath.Base(device)
		// ignore loop and ram devices
		if strings.HasPrefix(dev, "loop") || strings.HasPrefix(dev, "ram") {
			log.Debugf("ignoring loop or ram device: %s", dev)
			continue
		}
		dev = fmt.Sprintf("/dev/%s", dev)
		log.Debugf("checking device: %s", dev)
		// open readonly, ignore errors
		disk, err := diskfs.OpenWithMode(dev, diskfs.ReadOnly)
		if err != nil {
			log.Debugf("failed to open device read-only: %s: %v", dev, err)
			continue
		}
		fs, err := disk.GetFilesystem(0)
		if err != nil {
			log.Debugf("failed to get filesystem on partition 0 for device: %s: %v", dev, err)
			continue
		}
		// get the label
		label := strings.TrimSpace(fs.Label())
		log.Debugf("found trimmed filesystem label for device: %s: '%s'", dev, label)
		if label == "config-2" {
			log.Debugf("adding device: %s", dev)
			foundDevices = append(foundDevices, dev)
		}
	}
	return foundDevices
}

// NewConfigDrive returns a new ProviderConfigDrive
func NewConfigDrive(device string) *ProviderConfigDrive {
	mountPoint, err := ioutil.TempDir("", "cd")

	log.Debugf("one")
	p := ProviderConfigDrive{device, mountPoint, err, []byte{}, []byte{}}
	if err == nil {
		log.Debugf("twi")
		if p.err = p.mount(); p.err == nil {
			log.Debugf("tws")
			// read the userdata
			userdata, err := ioutil.ReadFile(path.Join(p.mountPoint, "openstack", "latest", configDriveUserdataFile))
			// did we find a file?
			if err == nil && userdata != nil {
				log.Debugf("twss")
				p.userdata = userdata
			}
			if p.userdata == nil {
				p.err = fmt.Errorf("no userdata found in ./openstack/latest/%v", configDriveUserdataFile)
			}
			// read the metadata
			metadata, err := ioutil.ReadFile(path.Join(p.mountPoint, "openstack", "latest", configDriveMetadataFile))
			// did we find a file?
			if err == nil && metadata != nil {
				p.metadata = metadata
			}
			p.unmount()
		}
	}
	return &p
}

func (p *ProviderConfigDrive) String() string {
	return "ConfigDrive " + p.device
}

// Probe checks if the CD has the right file
func (p *ProviderConfigDrive) Probe() bool {
	return len(p.userdata) != 0
}

// Extract gets both the ConfigDrive specific and generic userdata
func (p *ProviderConfigDrive) Extract() ([]byte, error) {
	return p.userdata, p.err
}

// mount mounts a ConfigDrive/DVD device under mountPoint
func (p *ProviderConfigDrive) mount() error {
	// We may need to poll a little for device ready
	return syscall.Mount(p.device, p.mountPoint, "iso9660", syscall.MS_RDONLY, "")
}

// unmount removes the mount
func (p *ProviderConfigDrive) unmount() {
	_ = syscall.Unmount(p.mountPoint, 0)
}
