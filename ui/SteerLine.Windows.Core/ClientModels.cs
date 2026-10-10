using System.Text.Json.Serialization;

namespace SteerLine.Windows.Core;

public static class UiLabels
{
    public const string ConfigPath = "服务配置文件";
    public const string ProjectPath = "项目目录";
    public const string Connect = "连接现有服务";
    public const string ConnectionStatus = "服务连接状态";
    public const string TaskList = "真实任务列表";
    public const string Input = "未发送的多行输入";
    public const string SubmitIntent = "记录本地提交意图";
    public const string IntentStatus = "本地提交意图状态";
}

public sealed record ServiceProcessIdentity(
    [property: JsonPropertyName("kind")] string Kind,
    [property: JsonPropertyName("pid")] int Pid,
    [property: JsonPropertyName("created")] string Created,
    [property: JsonPropertyName("executable")] string Executable);

public sealed record ServiceDiscovery(
    [property: JsonPropertyName("version")] int Version,
    [property: JsonPropertyName("protocol")] string Protocol,
    [property: JsonPropertyName("project_id")] string ProjectId,
    [property: JsonPropertyName("repository_identity")] string RepositoryIdentity,
    [property: JsonPropertyName("instance_id")] string InstanceId,
    [property: JsonPropertyName("process")] ServiceProcessIdentity Process,
    [property: JsonPropertyName("address")] string Address,
    [property: JsonPropertyName("credential_path")] string CredentialPath,
    [property: JsonPropertyName("ready")] bool Ready,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("diagnostic")] string? Diagnostic,
    [property: JsonPropertyName("lease_id")] string LeaseId,
    [property: JsonPropertyName("updated_at")] DateTimeOffset UpdatedAt);

public sealed record ServiceIdentity(
    [property: JsonPropertyName("protocol")] string Protocol,
    [property: JsonPropertyName("project_id")] string ProjectId,
    [property: JsonPropertyName("repository_identity")] string RepositoryIdentity,
    [property: JsonPropertyName("instance_id")] string InstanceId,
    [property: JsonPropertyName("process")] ServiceProcessIdentity Process,
    [property: JsonPropertyName("address")] string Address,
    [property: JsonPropertyName("ready")] bool Ready,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("capabilities")] IReadOnlyList<string> Capabilities,
    [property: JsonPropertyName("sampled_at")] DateTimeOffset SampledAt,
    [property: JsonPropertyName("diagnostic")] string? Diagnostic);

// The product TaskPage DTO keeps the repository identity in project_id; the HTTP route key is only in the URL.
public sealed record TaskPage(
    [property: JsonPropertyName("protocol")] string Protocol,
    [property: JsonPropertyName("project_id")] string RepositoryIdentity,
    [property: JsonPropertyName("authority_version")] ulong AuthorityVersion,
    [property: JsonPropertyName("graph_revision")] ulong GraphRevision,
    [property: JsonPropertyName("database_revision")] ulong DatabaseRevision,
    [property: JsonPropertyName("database_id")] string DatabaseId,
    [property: JsonPropertyName("sampled_at")] DateTimeOffset SampledAt,
    [property: JsonPropertyName("tasks")] IReadOnlyList<TaskSummary> Tasks,
    [property: JsonPropertyName("next_cursor")] string? NextCursor,
    [property: JsonPropertyName("has_more")] bool HasMore);

public sealed record TaskSummary(
    [property: JsonPropertyName("task_id")] string TaskId,
    [property: JsonPropertyName("title")] string Title,
    [property: JsonPropertyName("type")] string Type,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("version")] ulong Version,
    [property: JsonPropertyName("claimant")] string Claimant);

public sealed record TaskDetailPage(
    [property: JsonPropertyName("protocol")] string Protocol,
    [property: JsonPropertyName("project_id")] string RepositoryIdentity,
    [property: JsonPropertyName("authority_version")] ulong AuthorityVersion,
    [property: JsonPropertyName("graph_revision")] ulong GraphRevision,
    [property: JsonPropertyName("database_revision")] ulong DatabaseRevision,
    [property: JsonPropertyName("database_id")] string DatabaseId,
    [property: JsonPropertyName("sampled_at")] DateTimeOffset SampledAt,
    [property: JsonPropertyName("tasks")] IReadOnlyList<TaskDetail> Tasks);

public sealed record TaskDetail(
    [property: JsonPropertyName("task_id")] string TaskId,
    [property: JsonPropertyName("title")] string Title,
    [property: JsonPropertyName("type")] string Type,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("version")] ulong Version,
    [property: JsonPropertyName("claimant")] string Claimant,
    [property: JsonPropertyName("startup_blockers")] IReadOnlyList<string>? StartupBlockers,
    [property: JsonPropertyName("unmet_predecessors")] IReadOnlyList<DependencySummary>? UnmetPredecessors,
    [property: JsonPropertyName("successors")] IReadOnlyList<string>? Successors,
    [property: JsonPropertyName("dependency_ready")] bool DependencyReady);

public sealed record DependencySummary(
    [property: JsonPropertyName("task_id")] string TaskId,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("delivered")] bool Delivered,
    [property: JsonPropertyName("blocker")] string Blocker);

public sealed record LocalSubmitIntent(string ProjectId, string TargetId, string Text, DateTimeOffset CreatedAt);

public sealed class LocalServiceException(string code, string message) : Exception(message)
{
    public string Code { get; } = code;
}
