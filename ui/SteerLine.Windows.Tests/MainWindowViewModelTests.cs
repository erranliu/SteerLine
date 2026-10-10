using SteerLine.Windows.Core;

namespace SteerLine.Windows.Tests;

public sealed class MainWindowViewModelTests
{
    [Fact]
    public async Task ATargetSwitchDoesNotApplyALateDetailResponse()
    {
        var a = TaskSummaryFor("a");
        var b = TaskSummaryFor("b");
        var service = new FakeService("project-a", [a, b]);
        var lateB = new TaskCompletionSource<TaskDetailPage>(TaskCreationOptions.RunContinuationsAsynchronously);
        var lateA = new TaskCompletionSource<TaskDetailPage>(TaskCreationOptions.RunContinuationsAsynchronously);
        var aCalls = 0;
        service.ShowHandler = (id, _) => id switch
        {
            "a" when Interlocked.Increment(ref aCalls) == 1 => Task.FromResult(DetailPage("C:\\repo\\.git", Detail("a", 1))),
            "a" => lateA.Task,
            "b" => lateB.Task,
            _ => throw new InvalidOperationException("unexpected task")
        };
        var viewModel = ViewModel(service);
        await viewModel.ConnectAsync("config.json", "C:\\repo");
        await viewModel.SelectTaskAsync(a);

        var oldTargetRequest = viewModel.SelectTaskAsync(b);
        var currentTargetRequest = viewModel.SelectTaskAsync(a);
        lateA.SetResult(DetailPage("C:\\repo\\.git", Detail("a", 2)));
        await currentTargetRequest;
        lateB.SetResult(DetailPage("C:\\repo\\.git", Detail("b", 3)));
        await oldTargetRequest;

        Assert.Equal("a", viewModel.SelectedTask?.TaskId);
        Assert.Equal("a", viewModel.SelectedTaskDetail?.TaskId);
        Assert.Equal((ulong)2, viewModel.SelectedTaskDetail?.Version);
    }

    [Fact]
    public async Task RejectsTaskListFromAnotherProjectAndClearsItsRows()
    {
        var service = new FakeService("project-a", [TaskSummaryFor("a")]) { ListRepositoryIdentity = "C:\\wrong\\.git" };
        var viewModel = ViewModel(service);

        await viewModel.ConnectAsync("config.json", "C:\\repo");

        Assert.False(viewModel.IsConnected);
        Assert.Empty(viewModel.Tasks);
        Assert.Contains("identity_conflict", viewModel.ConnectionStatus, StringComparison.Ordinal);
    }

    [Fact]
    public async Task DraftsStayBoundToTheirProjectAndExactTarget()
    {
        var store = new InMemoryDraftStore();
        var service = new FakeService("project-a", [TaskSummaryFor("a"), TaskSummaryFor("b")]);
        var viewModel = new MainWindowViewModel(new FakeFactory(service), store);
        await viewModel.ConnectAsync("config.json", "C:\\repo");
        await viewModel.SelectTaskAsync(TaskSummaryFor("a"));
        viewModel.UpdateDraft("draft for a");
        await viewModel.SelectTaskAsync(TaskSummaryFor("b"));
        viewModel.UpdateDraft("draft for b");
        await viewModel.SelectTaskAsync(TaskSummaryFor("a"));

        Assert.Equal("draft for a", viewModel.CurrentDraft);
        Assert.Equal("draft for b", store.Get("project-a", "b"));
        store.Set("project-b", "a", "another project's draft");
        Assert.Equal("draft for a", store.Get("project-a", "a"));
        Assert.Equal("another project's draft", store.Get("project-b", "a"));
    }

    [Fact]
    public async Task TargetChangesBeforeAnAsyncDraftFlushCanAcceptNewText()
    {
        var a = TaskSummaryFor("a");
        var b = TaskSummaryFor("b");
        var service = new FakeService("project-a", [a, b]);
        var store = new DelayedDraftStore();
        var viewModel = new MainWindowViewModel(new FakeFactory(service), store);
        await viewModel.ConnectAsync("config.json", "C:\\repo");
        await viewModel.SelectTaskAsync(a);
        viewModel.UpdateDraft("draft for a");

        var switchTask = viewModel.SelectTaskAsync(b);
        Assert.Equal("b", viewModel.SelectedTask?.TaskId);
        viewModel.UpdateDraft("draft for b");
        store.ReleaseSave.SetResult();
        await switchTask;
        await viewModel.SelectTaskAsync(a);

        Assert.Equal("draft for a", viewModel.CurrentDraft);
        Assert.Equal("draft for b", store.Get("project-a", "b"));
    }

    [Fact]
    public async Task DraftFilesStayInsideTheirSelectedHome()
    {
        var parent = Directory.CreateTempSubdirectory("steerline-ui-drafts-").FullName;
        var homeA = Path.Combine(parent, "home-a");
        var homeB = Path.Combine(parent, "home-b");
        try
        {
            var store = new FileDraftStore();
            await store.LoadAsync(homeA, CancellationToken.None);
            store.Set("project-a", "task-1", "private to home A");
            await store.SaveAsync(CancellationToken.None);
            await store.LoadAsync(homeB, CancellationToken.None);

            Assert.Equal(string.Empty, store.Get("project-a", "task-1"));
            store.Set("project-a", "task-1", "private to home B");
            await store.SaveAsync(CancellationToken.None);

            var restoredA = new FileDraftStore();
            await restoredA.LoadAsync(homeA, CancellationToken.None);
            Assert.Equal("private to home A", restoredA.Get("project-a", "task-1"));
            Assert.Equal(string.Empty, restoredA.Get("project-a", "task-2"));
        }
        finally
        {
            Directory.Delete(parent, recursive: true);
        }
    }

    [Fact]
    public async Task ExplicitSubmitRecordsLocalIntentWithoutCallingAnyBusinessCommand()
    {
        var service = new FakeService("project-a", [TaskSummaryFor("a")]);
        var viewModel = ViewModel(service);
        await viewModel.ConnectAsync("config.json", "C:\\repo");
        Assert.False(viewModel.HasSelectedTarget);
        viewModel.RecordLocalIntent();
        Assert.Contains("选择一个实际任务", viewModel.IntentStatus, StringComparison.Ordinal);
        await viewModel.SelectTaskAsync(TaskSummaryFor("a"));
        viewModel.UpdateDraft("请继续检查");

        viewModel.RecordLocalIntent();

        Assert.Equal("请继续检查", viewModel.CurrentDraft);
        Assert.Contains("未发送业务命令", viewModel.IntentStatus, StringComparison.Ordinal);
        Assert.Equal(1, service.ListCalls);
        Assert.Equal(1, service.ShowCalls);
        var methodNames = typeof(IReadOnlyLocalService).GetMethods().Select(method => method.Name);
        Assert.DoesNotContain(methodNames, name => name.Contains("Command", StringComparison.OrdinalIgnoreCase) || name.Contains("Submit", StringComparison.OrdinalIgnoreCase) || name.StartsWith("Post", StringComparison.OrdinalIgnoreCase));
    }

    [Fact]
    public void AccessibleLabelsArePresentAndDistinct()
    {
        var labels = new[] { UiLabels.ConfigPath, UiLabels.ProjectPath, UiLabels.Connect, UiLabels.ConnectionStatus,
            UiLabels.TaskList, UiLabels.Input, UiLabels.SubmitIntent, UiLabels.IntentStatus };

        Assert.All(labels, label => Assert.False(string.IsNullOrWhiteSpace(label)));
        Assert.Equal(labels.Length, labels.Distinct(StringComparer.Ordinal).Count());
    }

    private static MainWindowViewModel ViewModel(FakeService service) => new(new FakeFactory(service), new InMemoryDraftStore());

    private static TaskSummary TaskSummaryFor(string id) => new(id, "Task " + id, "task", "open", 1, string.Empty);
    private static TaskDetail Detail(string id, ulong version) => new(id, "Task " + id, "task", "open", version, string.Empty, null, null, null, true);
    private static TaskDetailPage DetailPage(string projectId, TaskDetail detail) => new("native-tasks/v1", projectId, 1, 1, 1, "db", DateTimeOffset.UtcNow, [detail]);

    private sealed class FakeFactory(FakeService service) : ILocalServiceFactory
    {
        public Task<IReadOnlyLocalService> ConnectAsync(string configPath, string projectPath, CancellationToken cancellationToken) => Task.FromResult<IReadOnlyLocalService>(service);
    }

    private sealed class DelayedDraftStore : IDraftStore
    {
        private readonly Dictionary<(string ProjectId, string TargetId), string> _drafts = [];
        private bool _delayNextSave = true;
        public TaskCompletionSource ReleaseSave { get; } = new(TaskCreationOptions.RunContinuationsAsynchronously);
        public string Get(string projectId, string targetId) => _drafts.GetValueOrDefault((projectId, targetId), string.Empty);
        public void Set(string projectId, string targetId, string text) => _drafts[(projectId, targetId)] = text;
        public Task LoadAsync(string homeDirectory, CancellationToken cancellationToken) => Task.CompletedTask;
        public async Task SaveAsync(CancellationToken cancellationToken)
        {
            if (_delayNextSave)
            {
                _delayNextSave = false;
                await ReleaseSave.Task.WaitAsync(cancellationToken);
            }
        }
    }

    private sealed class FakeService(string projectId, IReadOnlyList<TaskSummary> tasks) : IReadOnlyLocalService
    {
        private int _showCalls;
        public ServiceIdentity Identity { get; } = new("steerline-client/v1", projectId, "C:\\repo\\.git", "instance-a",
            new ServiceProcessIdentity("os", 120, "01dd000000000000", "C:\\SteerLine\\steerline.exe"), "127.0.0.1:50000", true, "ready", [], DateTimeOffset.UtcNow, null);
        public string HomeDirectory => "C:\\home";
        public string? ListRepositoryIdentity { get; set; }
        public int ListCalls { get; private set; }
        public int ShowCalls => _showCalls;
        public Func<string, int, Task<TaskDetailPage>>? ShowHandler { get; set; }

        public Task<TaskPage> ListTasksAsync(CancellationToken cancellationToken)
        {
            ListCalls++;
            return Task.FromResult(new TaskPage("native-tasks/v1", ListRepositoryIdentity ?? "C:\\repo\\.git", 1, 1, 1, "db", DateTimeOffset.UtcNow, tasks, null, false));
        }

        public Task<TaskDetailPage> ShowTaskAsync(string taskId, CancellationToken cancellationToken)
        {
            var call = Interlocked.Increment(ref _showCalls);
            return ShowHandler is null ? Task.FromResult(DetailPage("C:\\repo\\.git", Detail(taskId, 1))) : ShowHandler(taskId, call);
        }

        public void Dispose() { }
    }
}
