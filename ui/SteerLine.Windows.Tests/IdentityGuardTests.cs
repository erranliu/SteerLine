using SteerLine.Windows.Core;

namespace SteerLine.Windows.Tests;

public sealed class IdentityGuardTests
{
    [Fact]
    public void AcceptsOnlyTheDiscoveredProjectAndInstance()
    {
        var process = new ServiceProcessIdentity("os", 120, "01dd000000000000", "C:\\SteerLine\\steerline.exe");
        var discovery = new ServiceDiscovery(1, "steerline-client/v1", "project-id", "C:\\repo\\.git", "instance-a", process,
            "127.0.0.1:50000", "C:\\state\\credentials\\token", true, "ready", null, "lease-a", DateTimeOffset.UtcNow);
        var live = new ServiceIdentity("steerline-client/v1", "project-id", "c:\\repo\\.git", "instance-a", process,
            "127.0.0.1:50000", true, "ready", [], DateTimeOffset.UtcNow, null);

        IdentityGuard.Verify(discovery, live, "project-id", "c:\\repo\\.git");
        Assert.True(IdentityGuard.RepositoryEquals("C:\\repo\\.git", live.RepositoryIdentity));
        Assert.False(IdentityGuard.RepositoryEquals("C:\\other\\.git", live.RepositoryIdentity));
    }

    [Theory]
    [InlineData("wrong-project", "instance-a")]
    [InlineData("project-id", "wrong-instance")]
    public void RejectsAResponseFromAnotherProjectOrInstance(string projectId, string instanceId)
    {
        var process = new ServiceProcessIdentity("os", 120, "01dd000000000000", "C:\\SteerLine\\steerline.exe");
        var discovery = new ServiceDiscovery(1, "steerline-client/v1", "project-id", "C:\\repo\\.git", "instance-a", process,
            "127.0.0.1:50000", "C:\\state\\credentials\\token", true, "ready", null, "lease-a", DateTimeOffset.UtcNow);
        var live = new ServiceIdentity("steerline-client/v1", projectId, "C:\\repo\\.git", instanceId, process,
            "127.0.0.1:50000", true, "ready", [], DateTimeOffset.UtcNow, null);

        var error = Assert.Throws<LocalServiceException>(() => IdentityGuard.Verify(discovery, live, "project-id", "C:\\repo\\.git"));
        Assert.Equal("identity_conflict", error.Code);
    }
}
