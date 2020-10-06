package qemu

import (
	"fmt"
	"testing"

	"github.com/hashicorp/packer/helper/communicator"
	"github.com/hashicorp/packer/helper/multistep"
	"github.com/hashicorp/packer/packer"
	"github.com/stretchr/testify/assert"
)

func runTestState(t *testing.T, config *Config) multistep.StateBag {
	state := new(multistep.BasicStateBag)
	state.Put("config", config)

	d := new(DriverMock)
	d.VersionResult = "3.0.0"
	state.Put("driver", d)

	state.Put("ui", packer.TestUi(t))

	state.Put("commHostPort", 5000)
	state.Put("floppy_path", "fake_floppy_path")
	state.Put("http_ip", "127.0.0.1")
	state.Put("http_port", 1234)
	state.Put("iso_path", "/path/to/test.iso")
	state.Put("qemu_disk_paths", []string{})
	state.Put("vnc_port", 5905)
	state.Put("vnc_password", "fake_vnc_password")

	return state
}

func Test_DriveAndDeviceArgs(t *testing.T) {
	type testCase struct {
		Config     *Config
		ExtraState map[string]interface{}
		Step       *stepRun
		Expected   []string
		Reason     string
	}

	testcases := []testCase{
		{
			&Config{},
			map[string]interface{}{},
			&stepRun{},
			[]string{
				"-display", "gtk",
				"-boot", "once=d",
				"-drive", "file=/path/to/test.iso,index=0,media=cdrom",
			},
			"Boot value should default to once=d with diskImage isnt set",
		},
		{
			&Config{
				DiskImage:     true,
				DiskInterface: "virtio-scsi",

				OutputDir: "/path/to/output",
				DiskCache: "writeback",
				Format:    "qcow2",
			},
			map[string]interface{}{
				"cd_path":         "fake_cd_path.iso",
				"qemu_disk_paths": []string{"qemupath1", "qemupath2"},
			},
			&stepRun{},
			[]string{
				"-display", "gtk",
				"-boot", "c",
				"-device", "virtio-scsi-pci,id=scsi0",
				"-device", "virtio-scsi-pci,id=scsi0",
				"-device", "virtio-scsi-pci,id=scsi0",
				"-device", "scsi-hd,bus=scsi0.0,drive=drive0",
				"-device", "scsi-hd,bus=scsi0.0,drive=drive1",
				"-device", "scsi-hd,bus=scsi0.0,drive=drive2",
				"-drive", "if=none,file=/path/to/output,id=drive0,cache=writeback,discard=,format=qcow2,detect-zeroes=",
				"-drive", "if=none,file=qemupath1,id=drive1,cache=writeback,discard=,format=qcow2,detect-zeroes=",
				"-drive", "if=none,file=qemupath2,id=drive2,cache=writeback,discard=,format=qcow2,detect-zeroes=",
				"-drive", "file=fake_cd_path.iso,index=0,media=cdrom",
			},
			"virtio-scsi interface. Note this is broken; " +
				"the scsi drive being addd always has the same bus. " +
				"We need to fix this.",
		},
		{
			&Config{
				DiskInterface: "virtio-scsi",

				OutputDir: "/path/to/output",
				DiskCache: "writeback",
				Format:    "qcow2",
			},
			map[string]interface{}{
				"cd_path":         "fake_cd_path.iso",
				"qemu_disk_paths": []string{"qemupath1", "qemupath2"},
			},
			&stepRun{},
			[]string{
				"-display", "gtk",
				"-boot", "once=d",
				"-device", "virtio-scsi-pci,id=scsi0",
				"-device", "virtio-scsi-pci,id=scsi0",
				"-device", "scsi-hd,bus=scsi0.0,drive=drive0",
				"-device", "scsi-hd,bus=scsi0.0,drive=drive1",
				"-drive", "if=none,file=qemupath1,id=drive0,cache=writeback,discard=,format=qcow2,detect-zeroes=",
				"-drive", "if=none,file=qemupath2,id=drive1,cache=writeback,discard=,format=qcow2,detect-zeroes=",
				"-drive", "file=/path/to/test.iso,index=0,media=cdrom",
				"-drive", "file=fake_cd_path.iso,index=1,media=cdrom",
			},
			"virtio-scsi interface, bootable iso, cdrom",
		},
		{
			&Config{},
			map[string]interface{}{
				"cd_path": "fake_cd_path.iso",
			},
			&stepRun{},
			[]string{
				"-display", "gtk",
				"-boot", "once=d",
				"-drive", "file=/path/to/test.iso,index=0,media=cdrom",
				"-drive", "file=fake_cd_path.iso,index=1,media=cdrom",
			},
			"cd_path is set and DiskImage is false",
		},
		{
			&Config{},
			map[string]interface{}{},
			&stepRun{},
			[]string{
				"-display", "gtk",
				"-boot", "once=d",
				"-drive", "file=/path/to/test.iso,index=0,media=cdrom",
			},
			"empty config",
		},
		{
			&Config{
				OutputDir:     "/path/to/output",
				DiskInterface: "virtio",
				DiskCache:     "writeback",
				Format:        "qcow2",
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{
				"-boot", "once=d",
				"-drive", "file=/path/to/output,if=virtio,cache=writeback,format=qcow2",
				"-drive", "file=/path/to/test.iso,index=0,media=cdrom",
			},
			"version less than 2",
		},
	}
	for _, tc := range testcases {
		state := runTestState(t, tc.Config)
		for k, v := range tc.ExtraState {
			state.Put(k, v)
		}
		// figure out boot config
		bootval := "c"
		for i, val := range tc.Expected {
			if val == "-boot" {
				bootval = tc.Expected[i+1]
			}
		}

		args, err := tc.Step.getCommandArgs(bootval, state)
		if err != nil {
			t.Fatalf("should not have an error getting args. Error: %s", err)
		}

		expected := append([]string{
			"-m", "0M",
			"-fda", "fake_floppy_path",
			"-name", "",
			"-netdev", "user,id=user.0,hostfwd=tcp::5000-:0",
			"-vnc", ":5905",
			"-machine", "type=,accel=",
			"-device", ",netdev=user.0"},
			tc.Expected...)

		assert.ElementsMatch(t, args, expected,
			fmt.Sprintf("%s, \nRecieved: %#v", tc.Reason, args))
	}
}

func Test_OptionalConfigOptionsGetSet(t *testing.T) {
	c := &Config{
		VNCUsePassword: true,
		QMPEnable:      true,
		QMPSocketPath:  "qmp_path",
		VMName:         "MyFancyName",
		MachineType:    "pc",
		Accelerator:    "hvf",
	}

	state := runTestState(t, c)
	step := &stepRun{}
	args, err := step.getCommandArgs("once=d", state)
	if err != nil {
		t.Fatalf("should not have an error getting args. Error: %s", err)
	}

	expected := []string{
		"-display", "gtk",
		"-m", "0M",
		"-boot", "once=d",
		"-fda", "fake_floppy_path",
		"-name", "MyFancyName",
		"-netdev", "user,id=user.0,hostfwd=tcp::5000-:0",
		"-vnc", ":5905,password",
		"-machine", "type=pc,accel=hvf",
		"-device", ",netdev=user.0",
		"-drive", "file=/path/to/test.iso,index=0,media=cdrom",
		"-qmp", "unix:qmp_path,server,nowait",
	}

	assert.ElementsMatch(t, args, expected, "password flag should be set, and d drive should be set: %s", args)
}

// Tests for presence of Packer-generated arguments. Doesn't test that
// arguments which shouldn't be there are absent.
func Test_Defaults(t *testing.T) {
	type testCase struct {
		Config     *Config
		ExtraState map[string]interface{}
		Step       *stepRun
		Expected   []string
		Reason     string
	}

	testcases := []testCase{
		{
			&Config{},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-boot", "once=d"},
			"Boot value should default to once=d",
		},
		{
			&Config{},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-boot", "c"},
			"Boot value should be set to c when DiskImage is set on step",
		},
		{
			&Config{
				QMPEnable:     true,
				QMPSocketPath: "/path/to/socket",
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-qmp", "unix:/path/to/socket,server,nowait"},
			"Args should contain -qmp when qmp_enable is set",
		},
		{
			&Config{
				QMPEnable: true,
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-qmp", "unix:,server,nowait"},
			"Args contain -qmp even when socket path isn't set, if qmp enabled",
		},
		{
			&Config{
				VMName: "partyname",
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-name", "partyname"},
			"Name is set from config",
		},
		{
			&Config{},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-name", ""},
			"Name is set from config, even when name is blank (which won't " +
				"happen for real thanks to defaulting in build prepare)",
		},
		{
			&Config{
				Accelerator: "none",
				MachineType: "fancymachine",
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-machine", "type=fancymachine"},
			"Don't add accelerator tag when no accelerator is set.",
		},
		{
			&Config{
				Accelerator: "kvm",
				MachineType: "fancymachine",
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-machine", "type=fancymachine,accel=kvm"},
			"Add accelerator tag when accelerator is set.",
		},
		{
			&Config{
				NetBridge: "fakebridge",
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-netdev", "bridge,id=user.0,br=fakebridge"},
			"Add netbridge tag when netbridge is set.",
		},
		{
			&Config{
				CommConfig: CommConfig{
					Comm: communicator.Config{
						Type: "none",
					},
				},
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-netdev", "user,id=user.0"},
			"No host forwarding when no net bridge and no communicator",
		},
		{
			&Config{
				CommConfig: CommConfig{
					Comm: communicator.Config{
						Type: "ssh",
						SSH: communicator.SSH{
							SSHPort: 4567,
						},
					},
				},
			},
			map[string]interface{}{
				"commHostPort": 1111,
			},
			&stepRun{},
			[]string{"-netdev", "user,id=user.0,hostfwd=tcp::1111-:4567"},
			"Host forwarding when a communicator is configured",
		},
		{
			&Config{
				VNCBindAddress: "1.1.1.1",
			},
			map[string]interface{}{
				"vnc_port": 5959,
			},
			&stepRun{},
			[]string{"-vnc", "1.1.1.1:5959"},
			"no VNC password should be set",
		},
		{
			&Config{
				VNCBindAddress: "1.1.1.1",
				VNCUsePassword: true,
			},
			map[string]interface{}{
				"vnc_port": 5959,
			},
			&stepRun{},
			[]string{"-vnc", "1.1.1.1:5959,password"},
			"VNC password should be set",
		},
		{
			&Config{
				MemorySize: 2345,
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-m", "2345M"},
			"Memory is set, with unit M",
		},
		{
			&Config{
				CpuCount: 2,
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-smp", "cpus=2,sockets=2"},
			"both cpus and sockets are set to config's CpuCount",
		},
		{
			&Config{
				CpuCount: 2,
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-smp", "cpus=2,sockets=2"},
			"both cpus and sockets are set to config's CpuCount",
		},
		{
			&Config{
				CpuCount: 2,
			},
			map[string]interface{}{
				"floppy_path": "/path/to/floppy",
			},
			&stepRun{},
			[]string{"-fda", "/path/to/floppy"},
			"floppy path should be set under fda flag, when it exists",
		},
		{
			&Config{
				Headless:          false,
				Display:           "fakedisplay",
				UseDefaultDisplay: false,
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-display", "fakedisplay"},
			"Display option should value config display",
		},
		{
			&Config{
				Headless: false,
			},
			map[string]interface{}{},
			&stepRun{},
			[]string{"-display", "gtk"},
			"Display option should default to gtk",
		},
	}

	for _, tc := range testcases {
		state := runTestState(t, tc.Config)
		for k, v := range tc.ExtraState {
			state.Put(k, v)
		}

		args, err := tc.Step.getCommandArgs("once=d", state)
		if err != nil {
			t.Fatalf("should not have an error getting args. Error: %s", err)
		}
		if !matchArgument(args, tc.Expected) {
			t.Fatalf("Couldn't find %#v in result. Got: %#v, Reason: %s",
				tc.Expected, args, tc.Reason)
		}
	}
}

// This test makes sure that arguments don't end up in the final boot command
// if they aren't configured in the config.
// func TestDefaultsAbsentValues(t *testing.T) {}
func matchArgument(actual []string, expected []string) bool {
	key := expected[0]
	for i, k := range actual {
		if key == k {
			if expected[1] == actual[i+1] {
				return true
			}
		}
	}
	return false
}
