/*
© 2026 AxonOps Limited. All rights reserved.

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

package export

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

// exportAdaptiveRepair fetches adaptive repair settings and returns one resource.
func exportAdaptiveRepair(ctx context.Context, client *axonops.Client, opts *Options) ([]Resource, error) {
	settings, err := client.GetAdaptiveRepair(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching adaptive repair: %w", err)
	}
	if settings == nil {
		return nil, nil
	}

	name := toDNSLabel(opts.ClusterName + "-adaptive-repair")

	active := settings.Active
	gcGrace := settings.GcGraceThreshold
	tableParallelism := settings.TableParallelism
	filterTWCS := settings.FilterTWCSTables
	segRetries := settings.SegmentRetries
	segTargetSizeMB := settings.SegmentTargetSizeMB
	maxSegPerTable := settings.MaxSegmentsPerTable

	obj := &alertsv1alpha1.AxonOpsAdaptiveRepair{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "alerts.axonops.com/v1alpha1",
			Kind:       "AxonOpsAdaptiveRepair",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: opts.Namespace,
		},
		Spec: alertsv1alpha1.AxonOpsAdaptiveRepairSpec{
			ConnectionRef:       opts.ConnectionName,
			ClusterName:         opts.ClusterName,
			ClusterType:         opts.ClusterType,
			Active:              &active,
			GcGraceThreshold:    &gcGrace,
			TableParallelism:    &tableParallelism,
			ExcludedTables:      settings.BlacklistedTables,
			FilterTWCSTables:    &filterTWCS,
			SegmentRetries:      &segRetries,
			SegmentTargetSizeMB: &segTargetSizeMB,
			SegmentTimeout:      settings.SegmentTimeout,
			MaxSegmentsPerTable: &maxSegPerTable,
		},
	}

	return []Resource{{Kind: "AxonOpsAdaptiveRepair", Name: name, Object: obj}}, nil
}

// exportScheduledRepairs fetches scheduled repairs and returns CRD resources.
func exportScheduledRepairs(ctx context.Context, client *axonops.Client, opts *Options) ([]Resource, error) {
	resp, err := client.GetScheduledRepairs(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching scheduled repairs: %w", err)
	}
	if resp == nil || len(resp.Repairs) == 0 {
		return nil, nil
	}

	var resources []Resource
	for _, entry := range resp.Repairs {
		if len(entry.Params) == 0 {
			continue
		}
		p := entry.Params[0]
		tag := p.Tag
		if tag == "" {
			tag = entry.ID
		}
		name := toDNSLabel(opts.ClusterName + "-" + tag)

		obj := &alertsv1alpha1.AxonOpsScheduledRepair{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "alerts.axonops.com/v1alpha1",
				Kind:       "AxonOpsScheduledRepair",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: opts.Namespace,
			},
			Spec: alertsv1alpha1.AxonOpsScheduledRepairSpec{
				ConnectionRef:       opts.ConnectionName,
				ClusterName:         opts.ClusterName,
				ClusterType:         opts.ClusterType,
				Tag:                 tag,
				ScheduleExpression:  p.ScheduleExpr,
				Keyspace:            p.Keyspace,
				Tables:              p.Tables,
				BlacklistedTables:   p.BlacklistedTables,
				Nodes:               p.Nodes,
				SpecificDataCenters: p.SpecificDataCenters,
				Parallelism:         p.Parallelism,
				Segmented:           p.Segmented,
				SegmentsPerNode:     p.SegmentsPerNode,
				Incremental:         p.Incremental,
				JobThreads:          p.JobThreads,
				PrimaryRange:        p.PrimaryRange,
				OptimiseStreams:     p.OptimiseStreams,
				SkipPaxos:           p.SkipPaxos,
				PaxosOnly:           p.PaxosOnly,
			},
		}
		resources = append(resources, Resource{Kind: "AxonOpsScheduledRepair", Name: name, Object: obj})
	}
	return resources, nil
}

// exportBackups fetches scheduled snapshots and returns AxonOpsBackup resources.
func exportBackups(ctx context.Context, client *axonops.Client, opts *Options) ([]Resource, error) {
	resp, err := client.GetScheduledSnapshots(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching backups: %w", err)
	}
	if resp == nil || len(resp.ScheduledSnapshots) == 0 {
		return nil, nil
	}

	var resources []Resource
	for _, snap := range resp.ScheduledSnapshots {
		if len(snap.Params) == 0 {
			continue
		}
		// The first element of Params is the backup config object containing BackupDetails
		var param axonops.SnapshotParam
		if err := json.Unmarshal(snap.Params[0], &param); err != nil {
			return nil, fmt.Errorf("parsing backup params for %s: %w", snap.ID, err)
		}
		if param.BackupDetails == "" {
			continue
		}
		// BackupDetails is itself a JSON string containing the full payload
		var payload axonops.BackupPayload
		if err := json.Unmarshal([]byte(param.BackupDetails), &payload); err != nil {
			return nil, fmt.Errorf("parsing backup details for %s: %w", snap.ID, err)
		}

		tag := payload.Tag
		if tag == "" {
			tag = snap.ID
		}
		name := toDNSLabel(opts.ClusterName + "-" + tag)

		// Reconstruct table references
		var tables []string
		for _, t := range payload.Tables {
			tables = append(tables, t.Name)
		}

		schedule := payload.Schedule
		obj := backupV1alpha1(name, opts, payload, tag, schedule, tables)
		resources = append(resources, Resource{Kind: "AxonOpsBackup", Name: name, Object: obj})
	}
	return resources, nil
}

func backupV1alpha1(
	name string, opts *Options, payload axonops.BackupPayload, tag string, schedule bool, tables []string,
) any {
	// Use raw map so we don't need to import a separate package here.
	// The YAML marshaller handles this the same way as a typed struct.
	spec := map[string]any{
		"connectionRef":      opts.ConnectionName,
		"clusterName":        opts.ClusterName,
		"clusterType":        opts.ClusterType,
		"tag":                tag,
		"datacenters":        payload.Datacenters,
		"schedule":           schedule,
		"scheduleExpression": payload.ScheduleExpr,
		"localRetention":     payload.LocalRetentionDuration,
		"timeout":            payload.Timeout,
	}
	if len(payload.Nodes) > 0 {
		spec["nodes"] = payload.Nodes
	}
	if len(payload.Keyspaces) > 0 {
		spec["keyspaces"] = payload.Keyspaces
	}
	if len(tables) > 0 {
		spec["tables"] = tables
	}
	if payload.Remote {
		remote := map[string]any{
			"type":      payload.RemoteType,
			"path":      payload.RemotePath,
			"retention": payload.RemoteRetentionDuration,
		}
		if payload.Transfers > 0 {
			remote["transfers"] = payload.Transfers
		}
		if payload.BWLimit != "" {
			remote["bwLimit"] = payload.BWLimit
		}
		if payload.TPSLimit > 0 {
			remote["tpsLimit"] = payload.TPSLimit
		}
		spec["remote"] = remote
	}

	return map[string]any{
		"apiVersion": "backups.axonops.com/v1alpha1",
		"kind":       "AxonOpsBackup",
		"metadata": map[string]any{
			"name":      name,
			"namespace": opts.Namespace,
		},
		"spec": spec,
	}
}
