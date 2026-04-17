param(
    [Parameter(Mandatory = $false)]
    [string]$payload,

    [Parameter(Mandatory = $false)]
    [string]$apiKey,

    [Parameter(Mandatory = $false)]
    [string]$jsmUrl,

    [Parameter(Mandatory = $false)]
    [string]$logLevel,

    [Parameter(Mandatory = $false)]
    [string]$jecNamedPipe,

    # Captures undefined parameters to prevent "parameter not found" errors
    [Parameter(ValueFromRemainingArguments = $true)]
    $ExtraArguments
)

# Define the message to be sent to the pipe
$jsonResponse = @"
{
    "status": "success",
    "message": "Payload processed successfully",
    "items_updated": ["abc", "def", "ghi"]
}
"@

# Extract the pipe name from the full path
$pipeName = $jecNamedPipe -split '\\' | Select-Object -Last 1

try {
    # Initialize the pipe client
    $pipeClient = New-Object System.IO.Pipes.NamedPipeClientStream(".", $pipeName, [System.IO.Pipes.PipeDirection]::Out)

    # Attempt to connect (with a 5-second timeout)
    $pipeClient.Connect(5000)

    try {
        $pipeWriter = New-Object System.IO.StreamWriter($pipeClient)
        $pipeWriter.AutoFlush = $true

        # Send the string message
        $pipeWriter.WriteLine($jsonResponse)
    }
    catch {
        Write-Warning ("Error writing to pipe: {0}" -f $_.Exception.Message)
    }
    finally {
        # Ensure resources are released
        if ($null -ne $pipeWriter) { $pipeWriter.Dispose() }
        if ($null -ne $pipeClient) { $pipeClient.Dispose() }
    }
}
catch {
    Write-Warning ("Unable to connect to pipe: $pipeName. Error: {0}" -f $_.Exception.Message)
}
