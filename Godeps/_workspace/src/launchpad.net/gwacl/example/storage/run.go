// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

// This is an example of how to use GWACL to interact with the Azure storage
// services API.
//
// Note that it is provided "as-is" and contains very little error handling.
// Real code should handle errors.

package main

import (
    "flag"
    "fmt"
    "io/ioutil"
    "launchpad.net/gwacl"
    "os"
    "strings"
)

func badOperationError() error {
    return fmt.Errorf("Must specify one of %v", operationNames)
}

// operation is something you can instruct this program to do, by specifying
// its name on the command line.
type operation struct {
    // name is the operation name as used on the command line.
    name string
    // description holds a description of what the operation does.
    description string
    // example illustrates how the operation is used.
    example string
    // requiredArgs lists the command-line options that are required for this
    // operation.
    requiredArgs []string
    // validate is an optional callback to perform more detailed checking on
    // the operation's arguments.
    validate func() error
    // implementation is a function that performs the operation.  If it fails,
    // it just panics.
    implementation func(gwacl.StorageContext)
}

// operations defines what operations are available to be invoked from the
// command line.
var operations = []operation{
    {
        name:           "listcontainers",
        description:    "Show existing storage containers",
        example:        "listcontainers",
        implementation: listcontainers,
    },
    {
        name:           "list",
        description:    "List files in a container",
        example:        "-container=<container> list",
        requiredArgs:   []string{"container"},
        implementation: list,
    },
    {
        name:         "containeracl",
        description:  "Set access on a container",
        example:      "-container=<container> -acl <container|blob|private> containeracl",
        requiredArgs: []string{"container", "key", "acl"},
        validate: func() error {
            if acl != "container" && acl != "blob" && acl != "private" {
                return fmt.Errorf(
                    "Usage: containeracl -container=<container> <container|blob|private>")
            }
            return nil
        },
        implementation: containeracl,
    },
    {
        name:           "getblob",
        description:    "Get a file from a container (it's returned on stdout)",
        example:        "-container=<container> -filename=<filename> getblob",
        requiredArgs:   []string{"container", "filename"},
        implementation: getblob,
    },
    {
        name:           "addblock",
        description:    "Upload a file to a block blob",
        example:        "-container=<container> -filename=<filename> addblock",
        requiredArgs:   []string{"key", "container", "filename"},
        implementation: addblock,
    },
    {
        name:           "deleteblob",
        description:    "Delete a blob",
        example:        "-container=<container> -filename=<filename> deleteblob",
        requiredArgs:   []string{"key", "container", "filename"},
        implementation: deleteblob,
    },
    {
        name:        "putblob",
        description: "Create an empty page blob",
        example: "-container=<container> -blobname=<blobname> -size=<bytes> " +
            "-blobtype=\"page\" putblob",
        requiredArgs:   []string{"key", "blobname", "blobtype", "container", "size"},
        implementation: putblob,
    },
    {
        name: "putpage",
        description: "Upload a file to a page blob's page.  The range parameters must " +
            "be (modulo 512)-(modulo 512 -1), eg: -pagerange=0-511",
        example: "-container=<container> -blobname=<blobname> -pagerange=<N-N> " +
            "-filename=<local file> putpage",
        requiredArgs:   []string{"key", "blobname", "container", "pagerange", "filename"},
        implementation: putpage,
    },
}

// operationsByName maps each opeation name to an operation struct that
// describes it.
var operationsByName map[string]operation

// operationNames lists just the names of the oeprations, in the order in which
// they are listed in "operations."
var operationNames []string

func init() {
    operationsByName = make(map[string]operation, len(operations))
    for _, op := range operations {
        operationsByName[op.name] = op
    }

    operationNames = make([]string, len(operations))
    for index, op := range operations {
        operationNames[index] = op.name
    }
}

// Variables set by command-line options.
var (
    help      bool
    account   string
    location  string
    key       string
    filename  string
    container string
    prefix    string
    blobname  string
    blobtype  string
    size      int
    pagerange string
    acl       string
)

// argumentGiven returns whether the named argument was specified on the
// command line.
func argumentGiven(name string) bool {
    // This is stupid.  There must be a way to ask the flag module directly!
    switch name {
    case "account":
        return account != ""
    case "location":
        return location != ""
    case "key":
        return key != ""
    case "container":
        return container != ""
    case "filename":
        return filename != ""
    case "prefix":
        return prefix != ""
    case "blobname":
        return blobname != ""
    case "blobtype":
        return blobtype != ""
    case "size":
        return size != 0
    case "pagerange":
        return pagerange != ""
    case "acl":
        return acl != ""
    }
    panic(fmt.Errorf("internal error: unknown command-line option: %s", name))
}

func getParams() (string, error) {
    flag.BoolVar(&help, "h", false, "Show usage and exit")

    flag.StringVar(&account, "account", "", "Storage account name")
    flag.StringVar(&location, "location", "", "Azure location, e.g. \"West US\", \"China East\", or \"North Europe\"")
    flag.StringVar(&key, "key", "", "A valid storage account key (base64 encoded), defaults to the empty string (i.e. anonymous access)")
    flag.StringVar(&container, "container", "", "Name of the container to use")
    flag.StringVar(&filename, "filename", "", "File containing blob/page to upload/download")
    flag.StringVar(&prefix, "prefix", "", "Prefix to match when listing blobs")
    flag.StringVar(&blobname, "blobname", "", "Name of blob in container")
    flag.StringVar(&blobtype, "blobtype", "", "Type of blob, 'page' or 'block'")
    flag.IntVar(&size, "size", 0, "Size of blob to create for a page 'putblob'")
    flag.StringVar(&pagerange, "pagerange", "", "When uploading to a page blob, this specifies what range in the blob. Use the format 'start-end', e.g. -pagerange 1024-2048")
    flag.StringVar(&acl, "acl", "", "When using 'containeracl', specify an ACL type")
    flag.Parse()

    if help {
        return "", nil
    }

    opName := flag.Arg(0)
    if opName == "" {
        return "", fmt.Errorf("No operation specified")
    }

    requiredArgs := []string{"account", "location"}
    for _, arg := range requiredArgs {
        if !argumentGiven(arg) {
            return "", fmt.Errorf("Must supply %q parameter.", arg)
        }
    }

    if len(flag.Args()) != 1 {
        return "", badOperationError()
    }

    op, isDefined := operationsByName[opName]
    if !isDefined {
        return "", badOperationError()
    }

    for _, arg := range op.requiredArgs {
        if !argumentGiven(arg) {
            return "", fmt.Errorf("%q requires these options: %v", op.name, op.requiredArgs)
        }
    }

    if op.validate != nil {
        err := op.validate()
        if err != nil {
            return "", err
        }
    }

    return op.name, nil
}

func Usage() {
    fmt.Fprintf(
        os.Stderr,
        "Usage:\n    %s [args] <%s>\n",
        os.Args[0],
        strings.Join(operationNames, "|"))
    flag.PrintDefaults()

    fmt.Fprintf(os.Stderr, `
    This is an example of how to interact with the Azure storage service.
    It is not a complete example but it does give a useful way to do do some
    basic operations.

    The -account param must always be supplied and -key must be supplied for
    operations that change things, (get these from the Azure web UI) otherwise
    anonymous access is made.  Additionally there are the following command
    invocation parameters:
    `)

    for _, op := range operations {
        fmt.Fprintf(os.Stderr, "\n    %s:\n        %s\n", op.description, op.example)
    }
}

func dumpError(err error) {
    if err != nil {
        fmt.Fprintf(os.Stderr, "ERROR:")
        fmt.Fprintf(os.Stderr, "%s\n", err)
    }
}

func listcontainers(storage gwacl.StorageContext) {
    res, e := storage.ListAllContainers()
    if e != nil {
        dumpError(e)
        return
    }
    for _, c := range res.Containers {
        // TODO: embellish with the other container data
        fmt.Println(c.Name)
    }
}

func containeracl(storage gwacl.StorageContext) {
    err := storage.SetContainerACL(&gwacl.SetContainerACLRequest{
        Container: container,
        Access:    acl,
    })
    dumpError(err)
}

func list(storage gwacl.StorageContext) {
    request := &gwacl.ListBlobsRequest{
        Container: container, Prefix: prefix}
    res, err := storage.ListAllBlobs(request)
    if err != nil {
        dumpError(err)
        return
    }
    for _, b := range res.Blobs {
        fmt.Printf("%s, %s, %s\n", b.ContentLength, b.LastModified, b.Name)
    }
}

func addblock(storage gwacl.StorageContext) {
    var err error
    file, err := os.Open(filename)
    if err != nil {
        dumpError(err)
        return
    }
    defer file.Close()

    err = storage.UploadBlockBlob(container, filename, file)
    if err != nil {
        dumpError(err)
        return
    }
}

func deleteblob(storage gwacl.StorageContext) {
    err := storage.DeleteBlob(container, filename)
    dumpError(err)
}

func getblob(storage gwacl.StorageContext) {
    var err error
    file, err := storage.GetBlob(container, filename)
    if err != nil {
        dumpError(err)
        return
    }
    data, err := ioutil.ReadAll(file)
    if err != nil {
        dumpError(err)
        return
    }
    os.Stdout.Write(data)
}

func putblob(storage gwacl.StorageContext) {
    err := storage.PutBlob(&gwacl.PutBlobRequest{
        Container: container,
        BlobType:  blobtype,
        Filename:  blobname,
        Size:      size,
    })
    dumpError(err)
}

func putpage(storage gwacl.StorageContext) {
    var err error
    file, err := os.Open(filename)
    if err != nil {
        dumpError(err)
        return
    }
    defer file.Close()

    var start, end int
    fmt.Sscanf(pagerange, "%d-%d", &start, &end)

    err = storage.PutPage(&gwacl.PutPageRequest{
        Container:  container,
        Filename:   blobname,
        StartRange: start,
        EndRange:   end,
        Data:       file,
    })
    if err != nil {
        dumpError(err)
        return
    }
}

func main() {
    flag.Usage = Usage
    var err error
    op, err := getParams()
    if err != nil {
        fmt.Fprintf(os.Stderr, "%s\n", err.Error())
        fmt.Fprintf(os.Stderr, "Use -h for help with using this program.\n")
        os.Exit(1)
    }
    if help {
        Usage()
        os.Exit(0)
    }

    storage := gwacl.StorageContext{
        Account:       account,
        Key:           key,
        AzureEndpoint: gwacl.GetEndpoint(location),
    }

    perform := operationsByName[op].implementation
    perform(storage)
}
