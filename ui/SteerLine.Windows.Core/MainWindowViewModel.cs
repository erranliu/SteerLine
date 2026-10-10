using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Runtime.CompilerServices;

namespace SteerLine.Windows.Core;

public sealed class MainWindowViewModel(ILocalServiceFactory serviceFactory, IDraftStore draftStore) : INotifyPropertyChanged, IAsyncDisposable
{
    private IReadOnlyLocalService? _service;
    private CancellationTokenSource? _selectionCancellation;
    private long _selectionGeneration;
    private TaskSummary? _selectedTask;
    private TaskDetail? _selectedTaskDetail;
    private string _connectionStatus = "请选择项目目录和服务配置。";
    private string _targetStatus = "尚未选择任务。";
    private string _currentDraft = string.Empty;
    private string _intentStatus = "输入内容保留在本机；记录意图不会发送业务命令。";
    private bool _isConnected;

    public event PropertyChangedEventHandler? PropertyChanged;
    public ObservableCollection<TaskSummary> Tasks { get; } = [];
    public ServiceIdentity? Identity => _service?.Identity;
    public string ConnectionStatus { get => _connectionStatus; private set => Set(ref _connectionStatus, value); }
    public string TargetStatus { get => _targetStatus; private set => Set(ref _targetStatus, value); }
    public string CurrentDraft { get => _currentDraft; private set => Set(ref _currentDraft, value); }
    public string IntentStatus { get => _intentStatus; private set => Set(ref _intentStatus, value); }
    public string ProjectId => _service?.Identity.ProjectId ?? string.Empty;
    public bool IsConnected { get => _isConnected; private set => Set(ref _isConnected, value); }
    public bool HasSelectedTarget => _service is not null && _selectedTask is not null;
    public TaskSummary? SelectedTask => _selectedTask;
    public TaskDetail? SelectedTaskDetail { get => _selectedTaskDetail; private set => Set(ref _selectedTaskDetail, value); }
    public string ConfigPathLabel => UiLabels.ConfigPath;
    public string ProjectPathLabel => UiLabels.ProjectPath;
    public string ConnectLabel => UiLabels.Connect;
    public string ConnectionStatusLabel => UiLabels.ConnectionStatus;
    public string TaskListLabel => UiLabels.TaskList;
    public string InputLabel => UiLabels.Input;
    public string SubmitIntentLabel => UiLabels.SubmitIntent;
    public string IntentStatusLabel => UiLabels.IntentStatus;

    public async Task ConnectAsync(string configPath, string projectPath, CancellationToken cancellationToken = default)
    {
        var generation = BeginSelection();
        var previous = _service;
        if (previous is not null && _selectedTask is not null)
            draftStore.Set(previous.Identity.ProjectId, _selectedTask.TaskId, CurrentDraft);
        _service = null;
        OnPropertyChanged(nameof(HasSelectedTarget));
        IsConnected = false;
        Tasks.Clear();
        _selectedTask = null;
        SelectedTaskDetail = null;
        CurrentDraft = string.Empty;
        TargetStatus = "尚未选择任务。";
        if (previous is not null)
            await SaveDraftsAsync().ConfigureAwait(true);
        previous?.Dispose();

        ConnectionStatus = "正在核验所选项目和现有服务身份…";
        try
        {
            var service = await serviceFactory.ConnectAsync(configPath, projectPath, cancellationToken);
            if (generation != _selectionGeneration)
            {
                service.Dispose();
                return;
            }
            _service = service;
            OnPropertyChanged(nameof(HasSelectedTarget));
            await draftStore.LoadAsync(service.HomeDirectory, cancellationToken);
            var page = await service.ListTasksAsync(cancellationToken);
            if (generation != _selectionGeneration || !ReferenceEquals(_service, service))
                return;
            TaskPageGuard.Verify(page, service.Identity.RepositoryIdentity);
            foreach (var task in page.Tasks)
                Tasks.Add(task);
            IsConnected = true;
            ConnectionStatus = service.Identity.Ready
                ? $"已连接 · {service.Identity.RepositoryIdentity} · 实例 {service.Identity.InstanceId} · PID {service.Identity.Process.Pid}"
                : $"服务身份已核验但未就绪 · {service.Identity.Status} · {service.Identity.Diagnostic ?? "无诊断"}";
        }
        catch (Exception exception)
        {
            if (generation != _selectionGeneration)
                return;
            _service?.Dispose();
            _service = null;
            OnPropertyChanged(nameof(HasSelectedTarget));
            IsConnected = false;
            Tasks.Clear();
            _selectedTask = null;
            SelectedTaskDetail = null;
            CurrentDraft = string.Empty;
            TargetStatus = "尚未选择任务。";
            ConnectionStatus = FormatError(exception);
        }
    }

    public async Task RefreshAsync(CancellationToken cancellationToken = default)
    {
        var service = _service;
        if (service is null)
        {
            ConnectionStatus = "未连接现有服务。";
            return;
        }
        var generation = BeginSelection();
        ConnectionStatus = "正在读取当前任务列表…";
        try
        {
            var page = await service.ListTasksAsync(_selectionCancellation!.Token);
            if (generation != _selectionGeneration || !ReferenceEquals(_service, service))
                return;
            TaskPageGuard.Verify(page, service.Identity.RepositoryIdentity);
            Tasks.Clear();
            foreach (var task in page.Tasks)
                Tasks.Add(task);
            ConnectionStatus = service.Identity.Ready
                ? $"已连接 · {service.Identity.RepositoryIdentity} · 实例 {service.Identity.InstanceId} · PID {service.Identity.Process.Pid}"
                : $"服务未就绪 · {service.Identity.Status} · {service.Identity.Diagnostic ?? "无诊断"}";
            if (_selectedTask is not null && Tasks.FirstOrDefault(t => t.TaskId == _selectedTask.TaskId) is { } refreshed)
                await SelectTaskAsync(refreshed, _selectionCancellation.Token);
            else if (_selectedTask is not null)
                await SelectTaskAsync(null, _selectionCancellation.Token);
            else
            {
                SelectedTaskDetail = null;
                CurrentDraft = string.Empty;
                TargetStatus = "尚未选择任务。";
            }
        }
        catch (Exception exception)
        {
            if (generation != _selectionGeneration)
                return;
            Tasks.Clear();
            SelectedTaskDetail = null;
            ConnectionStatus = FormatError(exception);
        }
    }

    public async Task SelectTaskAsync(TaskSummary? task, CancellationToken cancellationToken = default)
    {
        var service = _service;
        var previous = _selectedTask;
        var savePreviousDraft = false;
        if (service is not null && previous is not null)
        {
            draftStore.Set(service.Identity.ProjectId, previous.TaskId, CurrentDraft);
            savePreviousDraft = true;
        }

        var generation = BeginSelection();
        _selectedTask = task;
        OnPropertyChanged(nameof(SelectedTask));
        OnPropertyChanged(nameof(HasSelectedTarget));
        SelectedTaskDetail = null;
        CurrentDraft = task is null || service is null ? string.Empty : draftStore.Get(service.Identity.ProjectId, task.TaskId);
        if (savePreviousDraft)
            await SaveDraftsAsync().ConfigureAwait(true);
        if (task is null || service is null)
        {
            TargetStatus = "尚未选择任务。";
            return;
        }

        TargetStatus = $"正在核验目标 Task {task.TaskId}…";
        try
        {
            var page = await service.ShowTaskAsync(task.TaskId, _selectionCancellation!.Token);
            if (generation != _selectionGeneration || !ReferenceEquals(_service, service) || _selectedTask?.TaskId != task.TaskId)
                return;
            TaskPageGuard.Verify(page, service.Identity.RepositoryIdentity, task.TaskId);
            SelectedTaskDetail = page.Tasks[0];
            var blockers = (SelectedTaskDetail.StartupBlockers ?? []).Concat((SelectedTaskDetail.UnmetPredecessors ?? []).Select(p => p.Blocker))
                .Where(s => !string.IsNullOrWhiteSpace(s));
            var blockerText = string.Join("；", blockers);
            TargetStatus = blockerText.Length == 0
                ? $"Task {task.TaskId} · {SelectedTaskDetail.Status} · 版本 {SelectedTaskDetail.Version}"
                : $"Task {task.TaskId} · {SelectedTaskDetail.Status} · 阻塞：{blockerText}";
        }
        catch (Exception exception)
        {
            if (generation != _selectionGeneration)
                return;
            SelectedTaskDetail = null;
            TargetStatus = $"目标查询失败：{FormatError(exception)}";
        }
    }

    public void UpdateDraft(string text)
    {
        CurrentDraft = text;
        if (_service is not null && _selectedTask is not null)
            draftStore.Set(_service.Identity.ProjectId, _selectedTask.TaskId, text);
    }

    public void RecordLocalIntent()
    {
        if (_service is null || _selectedTask is null)
        {
            IntentStatus = "未记录：请先连接服务并选择一个实际任务。";
            return;
        }
        if (string.IsNullOrWhiteSpace(CurrentDraft))
        {
            IntentStatus = "未记录：输入内容为空。";
            return;
        }
        var intent = new LocalSubmitIntent(_service.Identity.ProjectId, _selectedTask.TaskId, CurrentDraft, DateTimeOffset.UtcNow);
        IntentStatus = $"本地提交意图已记录 · Task {intent.TargetId} · {intent.CreatedAt.ToLocalTime():HH:mm:ss} · 未发送业务命令";
    }

    public void SetSelectionError(string message) => ConnectionStatus = $"项目配置错误 · {message}";

    public async ValueTask DisposeAsync()
    {
        BeginSelection();
        if (_service is not null && _selectedTask is not null)
            draftStore.Set(_service.Identity.ProjectId, _selectedTask.TaskId, CurrentDraft);
        await SaveDraftsAsync().ConfigureAwait(false);
        _service?.Dispose();
        _service = null;
        OnPropertyChanged(nameof(HasSelectedTarget));
        IsConnected = false;
    }

    private long BeginSelection()
    {
        _selectionCancellation?.Cancel();
        _selectionCancellation?.Dispose();
        _selectionCancellation = new CancellationTokenSource();
        return ++_selectionGeneration;
    }

    private static string FormatError(Exception exception) => exception is LocalServiceException local
        ? $"{local.Code} · {local.Message}"
        : $"连接失败 · {exception.Message}";

    private async Task SaveDraftsAsync()
    {
        try { await draftStore.SaveAsync(CancellationToken.None).ConfigureAwait(true); }
        catch (Exception exception) { IntentStatus = $"草稿保存失败：{exception.Message}"; }
    }

    private void Set<T>(ref T field, T value, [CallerMemberName] string? propertyName = null)
    {
        if (EqualityComparer<T>.Default.Equals(field, value)) return;
        field = value;
        OnPropertyChanged(propertyName);
    }

    private void OnPropertyChanged([CallerMemberName] string? propertyName = null) => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(propertyName));
}
