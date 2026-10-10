using SteerLine.Windows.Core;

namespace SteerLine.Windows.Tests;

public sealed class TaskPageGuardTests
{
    [Fact]
    public void RejectsUnsupportedTaskDetailProtocol()
    {
        var task = new TaskDetail("44", "WinUI", "feature", "open", 1, "", [], [], [], true);
        var page = new TaskDetailPage("native-tasks/v2", "C:\\repo\\.git", 1, 1, 1, "db", DateTimeOffset.UtcNow, [task]);

        var error = Assert.Throws<LocalServiceException>(() => TaskPageGuard.Verify(page, "C:\\repo\\.git", "44"));

        Assert.Equal("identity_conflict", error.Code);
    }

    [Fact]
    public void AcceptsMatchingTaskDetailProtocolRepositoryAndTarget()
    {
        var task = new TaskDetail("44", "WinUI", "feature", "open", 1, "", [], [], [], true);
        var page = new TaskDetailPage("native-tasks/v1", "C:\\repo\\.git", 1, 1, 1, "db", DateTimeOffset.UtcNow, [task]);

        TaskPageGuard.Verify(page, "c:\\repo\\.git", "44");
    }
}
