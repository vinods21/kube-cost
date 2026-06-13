package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/wellknownimports"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	protoRoot := filepath.Join(root, "proto")
	outputRoot := filepath.Join(protoRoot, "gen", "go")

	sources, err := sourceFiles(protoRoot)
	if err != nil {
		return err
	}

	compiler := protocompile.Compiler{
		Resolver: wellknownimports.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{protoRoot},
		}),
	}
	files, err := compiler.Compile(context.Background(), sources...)
	if err != nil {
		return fmt.Errorf("compile protobuf sources: %w", err)
	}

	if err := os.RemoveAll(filepath.Join(outputRoot, "cost")); err != nil {
		return fmt.Errorf("clean generated protobuf output: %w", err)
	}
	request, err := generatorRequest(files, sources)
	if err != nil {
		return err
	}
	for _, plugin := range []string{"protoc-gen-go", "protoc-gen-go-grpc"} {
		if err := invokePlugin(root, outputRoot, plugin, request); err != nil {
			return err
		}
	}
	return nil
}

func sourceFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(root, "cost"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".proto" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	sort.Strings(files)
	return files, err
}

func generatorRequest(files linker.Files, sources []string) (*pluginpb.CodeGeneratorRequest, error) {
	descriptors := make(map[string]protoreflect.FileDescriptor)
	var ordered []protoreflect.FileDescriptor
	var collect func(protoreflect.FileDescriptor)
	collect = func(file protoreflect.FileDescriptor) {
		if _, exists := descriptors[file.Path()]; exists {
			return
		}
		descriptors[file.Path()] = file
		imports := file.Imports()
		for i := 0; i < imports.Len(); i++ {
			imported := imports.Get(i)
			if imported.FileDescriptor != nil {
				collect(imported.FileDescriptor)
			}
		}
		ordered = append(ordered, file)
	}
	for _, file := range files {
		collect(file)
	}

	request := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: sources,
		Parameter:      proto.String("paths=source_relative"),
	}
	for _, descriptor := range ordered {
		request.ProtoFile = append(request.ProtoFile, protodesc.ToFileDescriptorProto(descriptor))
	}
	return request, nil
}

func invokePlugin(root, outputRoot, plugin string, request *pluginpb.CodeGeneratorRequest) error {
	pluginPath, err := toolPath(root, plugin)
	if err != nil {
		return err
	}
	input, err := proto.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal request for %s: %w", plugin, err)
	}

	command := exec.Command(pluginPath)
	command.Dir = root
	command.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed: %w: %s", plugin, err, strings.TrimSpace(stderr.String()))
	}

	response := new(pluginpb.CodeGeneratorResponse)
	if err := proto.Unmarshal(stdout.Bytes(), response); err != nil {
		return fmt.Errorf("decode %s response: %w", plugin, err)
	}
	if response.Error != nil {
		return errors.New(response.GetError())
	}
	for _, file := range response.File {
		target := filepath.Join(outputRoot, filepath.FromSlash(file.GetName()))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(file.GetContent()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func toolPath(root, tool string) (string, error) {
	command := exec.Command("go", "tool", "-n", tool)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", tool, err)
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", fmt.Errorf("resolve %s: empty tool path", tool)
	}
	return path, nil
}
