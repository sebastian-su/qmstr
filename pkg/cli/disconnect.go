package cli

import (
	"fmt"
	"reflect"

	"github.com/QMSTR/qmstr/pkg/service"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

var disconnectCmdFlags = struct {
	edge string
}{}
var deleteNodeMsg *service.DeleteMessage

var disconnectCmd = &cobra.Command{
	Use:   "disconnect [type_of_node:attribute:value] [type_of_node:attribute:value]",
	Short: "Disconnect nodes with specific edges",
	Long: `Usage: qmstrctl disconnect <that> <this>...
Disconnect from Node <that> Node(s) <this>.`,
	Run: func(cmd *cobra.Command, args []string) {
		setUpControlService()
		setUpBuildService()
		if err := disconnectNodes(cmd, args); err != nil {
			Log.Fatalf("Disonnect failed: %v", err)
		}
		tearDownServer()
	},
	Args: cobra.MinimumNArgs(2),
}

func init() {
	rootCmd.AddCommand(disconnectCmd)
	disconnectCmd.Flags().StringVar(&disconnectCmdFlags.edge, "edge", "", "Edge to use when disconnecting nodes")
}

func disconnectNodes(cmd *cobra.Command, args []string) error {
	thatID, err := ParseNodeID(args[0])
	if err != nil {
		return fmt.Errorf("ParseNodeID fail for %q: %v", args[1], err)
	}
	switch thatVal := thatID.(type) {
	case *service.FileNode:
		that, err := getUniqueFileNode(thatVal)
		if err != nil {
			return err
		}
		err = disconnectFromFileNode(that, args[1:])
		if err != nil {
			return fmt.Errorf("disconnect from file node fail: %v", err)
		}
	case *service.PackageNode:
		that, err := controlServiceClient.GetPackageNode(context.Background(), &service.PackageNode{})
		if err != nil {
			return fmt.Errorf("get package node fail: %v", err)
		}
		err = disconnectFromPackageNode(that, args[1:])
		if err != nil {
			return fmt.Errorf("disconnect from package node fail: %v", err)
		}
	default:
		return fmt.Errorf("unsuported node type %T", thatVal)
	}
	return nil
}

func disconnectFromFileNode(node *service.FileNode, args []string) error {
	for _, nID := range args {
		thisID, err := ParseNodeID(nID)
		if err != nil {
			return fmt.Errorf("ParseNodeID fail for %q: %v", args[0], err)
		}
		switch thisVal := thisID.(type) {
		// FileNode -> FileNode
		case *service.FileNode:
			this, err := getUniqueFileNode(thisVal)
			if err != nil {
				return err
			}
			// default edge
			if disconnectCmdFlags.edge == "" {
				disconnectCmdFlags.edge = "derivedFrom"
			}
			// Which edge
			switch disconnectCmdFlags.edge {
			case "derivedFrom":
				deleteNodeMsg = &service.DeleteMessage{Uid: node.Uid, Edge: "derivedFrom"}
				for i, dr := range node.DerivedFrom {
					if reflect.DeepEqual(this, dr) {
						node.DerivedFrom = append(node.DerivedFrom[:i], node.DerivedFrom[i+1:]...)
					}
				}
			case "dependencies":
				deleteNodeMsg = &service.DeleteMessage{Uid: node.Uid, Edge: "dependencies"}
				for i, dep := range node.Dependencies {
					if dep == this {
						node.Dependencies = append(node.Dependencies[:i], node.Dependencies[i+1:]...)
					}
				}
			default:
				return fmt.Errorf("unknown edge %q for FileNode -> FileNode. Valid values %v", disconnectCmdFlags.edge, validFileToFileEdges)
			}
		default:
			return fmt.Errorf("cannot disconnect %T from FileNode", thisVal)
		}
	}
	// delete edge
	res, err := buildServiceClient.DeleteEdge(context.Background(), deleteNodeMsg)
	if err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("deleting predicate fail: %v", err)
	}

	// ship node back with the modified edge
	err = sendFileNode(node)
	if err != nil {
		return fmt.Errorf("sending FileNode fail: %v", err)
	}
	return nil
}

func disconnectFromPackageNode(node *service.PackageNode, args []string) error {
	for _, nID := range args {
		thisID, err := ParseNodeID(nID)
		if err != nil {
			return fmt.Errorf("ParseNodeID fail for %q: %v", args[0], err)
		}
		switch thisVal := thisID.(type) {
		// FileNode -> PackageNode
		case *service.FileNode:
			this, err := getUniqueFileNode(thisVal)
			if err != nil {
				return err
			}
			// default edge
			if disconnectCmdFlags.edge == "" {
				disconnectCmdFlags.edge = "targets"
			}
			// Which edge
			switch disconnectCmdFlags.edge {
			case "targets":
				deleteNodeMsg = &service.DeleteMessage{Uid: node.Uid, Edge: "targets"}
				for i, target := range node.Targets {
					if reflect.DeepEqual(this, target) {
						node.Targets = append(node.Targets[:i], node.Targets[i+1:]...)
					}
				}
			default:
				return fmt.Errorf("unknown edge %q for FileNode -> PackageNode. Valid values %v", disconnectCmdFlags.edge, validFileToPackageEdges)
			}
		default:
			return fmt.Errorf("cannot disconnect %T from PackageNode", thisVal)
		}
		// delete edge
		res, err := buildServiceClient.DeleteEdge(context.Background(), deleteNodeMsg)
		if err != nil {
			return err
		}
		if !res.Success {
			return fmt.Errorf("deleting predicate fail: %v", err)
		}
	}
	stream, err := buildServiceClient.Package(context.Background())
	if err != nil {
		return err
	}
	// ship back modified targets
	for _, target := range node.Targets {
		err = stream.Send(target)
		if err != nil {
			return fmt.Errorf("send fileNode to pkg stream fail: %v", err)
		}
	}
	res, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("close stream fail: %v", err)
	}
	if !res.Success {
		return fmt.Errorf("sending node fail: %v", err)
	}
	return nil
}
