# Índice separado de pruebas Go

Se conservan los 25 archivos de prueba. No se ejecutaron ni borraron durante esta reorganización.

## Ubicación física y vista de trabajo

Los tests que acceden a funciones privadas deben seguir en su paquete Go. Por eso no se trasladan a un paquete general `tests`: cambiaría sus imports y rompería el acceso. La prueba de consola se movió junto con su paquete a `tools/local-demo/console`.

`.vscode/settings.json` oculta `**/*_test.go` únicamente del explorador. Para mostrarlos, cambiar esa entrada de `true` a `false` o usar los enlaces siguientes. Go y Git siguen reconociéndolos. El panel Testing puede usarse si está instalada la extensión Go.

## Responsabilidades cubiertas

- Agente/configuración: arranque, persistencia protegida, cierre y selector de backend.
- Control: protocolo, sincronización, diario, WebSocket, comandos, reproducción y Desktop.
- Engine/player/decoder: independencia de zonas, transporte, cargas y formatos.
- Audio: backend, mezcla, PCM, ring buffer y hardware ASIO.
- Demo: consola interactiva, separada del agente beta.

## Archivos y casos existentes

### [cmd/agent/desktop_test.go](../cmd/agent/desktop_test.go)

- `TestDesktopStopsOnCommandAndParentExit`
- `TestDesktopWaitsUntilStop`
- `TestDesktopObserverWritesJSON`
- `TestDesktopReadsPlaybackAndRejectsMalformedInput`

### [cmd/agent/desktop_windows_test.go](../cmd/agent/desktop_windows_test.go)

- `TestDesktopConfigurationKeepsCredentialPrivateAndReusable`

### [cmd/agent/main_test.go](../cmd/agent/main_test.go)

- `TestBackendSpecificConfiguration`

### [cmd/agent/switch_windows_test.go](../cmd/agent/switch_windows_test.go)

- `TestBackendChangeRequiresStoppedEngine`

### [internal/audio/asio/hardware_test.go](../internal/audio/asio/hardware_test.go)

- `TestRealDVSStreamingOptional`

### [internal/audio/asio/output_test.go](../internal/audio/asio/output_test.go)

- `TestOutputValidatesDeviceChannelsAndStartsOneMultichannelStream`
- `TestListInstalledASIODrivers`
- `TestOpenInstalledASIODriverOptional`
- `TestOutputRejectsInsufficientChannelsAndUnsupportedBuffer`
- `TestOutputReportsStartupAndShutdownErrors`
- `TestConfigRejectsInvalidValues`
- `TestBufferSizeValidation`
- `TestCallbackUnderrunWritesSilence`
- `testOutput`
- `testPCM`

### [internal/audio/asio/pcm_test.go](../internal/audio/asio/pcm_test.go)

- `TestPCMConverterPreservesInterleavedSamplesAndPadsChannels`
- `TestPCMConverterResamplesAtConfiguredRate`
- `TestPCMConverterRejectsPartialFrames`

### [internal/audio/asio/ring_test.go](../internal/audio/asio/ring_test.go)

- `TestFrameRingInterleavedToPlanar`
- `TestFrameRingUnderrunIsSilenceAndOverflowDropsNewest`
- `TestFrameRingConcurrentSingleProducerConsumer`

### [internal/audio/backend/factory_test.go](../internal/audio/backend/factory_test.go)

- `TestFromEnvironmentDefaultsToASIOAndRejectsInvalidBackend`
- `TestMissingASIODriverDoesNotFallBackToOto`
- `TestFromEnvironmentBuildsASIOConfiguration`
- `TestFromEnvironmentRejectsInvalidASIOValues`
- `TestPersistentFactoryNoFallback`

### [internal/audio/mixer/router_test.go](../internal/audio/mixer/router_test.go)

- `TestRoutesSimultaneousStereoZonesToInterleavedFrame`
- `TestClearingRoutesKeepsContinuousSilenceAndCanRestoreThem`
- `TestMonoRoutingSilenceVolumePauseAndEndIsolation`
- `TestRouterAcceptsPartialSourceReads`
- `TestConfigureRejectsInvalidAndOverlappingRoutes`
- `TestConcurrentReadsAndZoneControlsAndClose`

### [internal/audio/oto/output_test.go](../internal/audio/oto/output_test.go)

- `TestUnsupportedFormatAndClosedOutput`

### [internal/control/backend_test.go](../internal/control/backend_test.go)

- `TestOtoSharedZonesRejectPhysicalRoutes`

### [internal/control/backend_transport_test.go](../internal/control/backend_transport_test.go)

- `TestBackendTransportWithoutHardware`

### [internal/control/control_test.go](../internal/control/control_test.go)

- `testConfig`
- `TestAtomicConfigurationAndReports`
- `TestJournalRecoversCompletedAndUnknownWithoutReplay`
- `TestLostAcknowledgmentReusesDurableResult`
- `TestJournalCorruptionAndExclusiveAccess`
- `TestCommandFailures`
- `TestClientRejectsUnsafeURLsAndRedirects`
- `TestWebSocketPrivateAuthPingAndReconnect`
- `TestRuntimeRestoresChangesAfterWebSocketReconnect`

### [internal/control/desktop_playback_test.go](../internal/control/desktop_playback_test.go)

- `TestDesktopPlaybackAndServerCommandsShareRuntime`

### [internal/control/desktop_test.go](../internal/control/desktop_test.go)

- `TestDesktopSnapshotIncludesStateWithoutSignedURLs`

### [internal/control/laravel_test.go](../internal/control/laravel_test.go)

- `TestLaravelAudioEndToEnd`

### [internal/control/playback_test.go](../internal/control/playback_test.go)

- `TestValidateAudioAcceptsDistinctStereoAndMonoRoutes`
- `TestApplyAudioPropagatesZoneChannelAssignments`
- `TestSpecificSongSelectionAndInvalidSelectionPreservesPending`
- `TestPlayback403RenewsOnlyOnceAndPersistsFailure`
- `TestPendingDownloadCanBeStoppedByNextCommand`
- `TestAudioConfigurationIsAtomicAndRemovesZones`
- `TestPlaybackRecoveryAndTransportIntegration`

### [internal/decoder/https_test.go](../internal/decoder/https_test.go)

- `TestHTTPSPolicy`
- `TestHTTPSLimitsAndTimeout`
- `TestHTTPSRejectsDifferentContentVersion`

### [internal/engine/manager_test.go](../internal/engine/manager_test.go)

- `TestZonesAndLifecycle`
- `TestReconfigurePreservesExistingZoneAndClosesRemovedHandle`
- `TestRoutingRequiresCompleteNonOverlappingZoneAssignments`
- `TestMonitorWithoutConsoleAndErrorIsolation`
- `TestIndependentPlaybackIntegration`

### [internal/localconfig/config_windows_test.go](../internal/localconfig/config_windows_test.go)

- `TestCredentialPersistence`
- `TestExclusiveLock`
- `TestValidation`
- `TestRestrictedDirectory`
- `TestBackendSwitchPersistsAndPreservesASIO`

### [internal/player/https_test.go](../internal/player/https_test.go)

- `TestRemoteCancellationAndFailure`
- `TestHTTPSPlaybackIntegration`

### [internal/player/mono_test.go](../internal/player/mono_test.go)

- `TestMonoMixHandlesClippingAndPartialReads`

### [internal/player/player_test.go](../internal/player/player_test.go)

- `TestMissingAndInvalidMP3`
- `TestPlaybackIntegration`
- `TestVolumeAndEmptyList`
- `TestNavigationIntegration`

### [tools/local-demo/console/console_test.go](../tools/local-demo/console/console_test.go)

- `TestCommands`
- `TestEOF`

## Ejecución futura, cuando se autorice

```powershell
go test ./...
go test ./internal/control
```

Comandos documentados, no ejecutados en esta tarea. Algunas pruebas requieren Windows, hardware o integración externa; revisar sus condiciones antes de ejecutarlas.
