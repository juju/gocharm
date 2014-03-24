// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package gwacl

// This is a pre-defined, base64-encoded 512-byte footer for a Virtual Hard
// Disk (VHD) file.  The footer sets the size of the VHD to a fixed 20Mb
// (20972032 bytes) and is intended to be uploaded as the last page in a blob
// of that size.
//
// The end use of this is to have a quick way of defining a fixed-size VHD
// that can be attached to a VM instance.  The rest of the file can be
// sparse-filled as necessary with a filesystem to create a final, valid,
// mountable disk.
//
// In case you were wondering *why* you would want to do this, it's the only
// way of making additional data available to a new instance at boot time.
//
// If you want to generate a new one of these (if you need a new size for
// example), the easiest way is to use VirtualBox to define a new one, and
// then do 'tail -c 512 | base64' on that file.

const VHD_SIZE = 20972032 // This is 20Mib + 512 bytes
const VHD_FOOTER = `
Y29uZWN0aXgAAAACAAEAAP//////////GVKuuHZib3gABAACV2kyawAAAAABQAAAAAAAAAFAAAAC
WgQRAAAAAv//5y4OEjVapHY7QpuodZNf77j6AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=`
