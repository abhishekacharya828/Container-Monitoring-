package main

import (
    "fmt"
    "os/exec"
    "strings"
    "time"
    "net/http"
    "strconv"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    containerCPUUsage = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "docker_container_cpu_usage_percent",
            Help: "CPU usage in percentage",
        },
        []string{"name", "id"},
    )
    containerMemoryUsage = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "docker_container_memory_usage_percent",
            Help: "Memory usage in percentage",
        },
        []string{"name", "id"},
    )
    containerNetworkReceived = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "docker_container_network_received_bytes",
            Help: "Network received bytes",
        },
        []string{"name", "id"},
    )
    containerNetworkTransmitted = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "docker_container_network_transmitted_bytes",
            Help: "Network transmitted bytes",
        },
        []string{"name", "id"},
    )
    containerStatus = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "docker_container_status",
            Help: "Container status (1: running, 0: stopped)",
        },
        []string{"name", "id"},
    )
)

func init() {
    prometheus.MustRegister(containerCPUUsage)
    prometheus.MustRegister(containerMemoryUsage)
    prometheus.MustRegister(containerNetworkReceived)
    prometheus.MustRegister(containerNetworkTransmitted)
    prometheus.MustRegister(containerStatus)
}

func recordMetrics() {
    go func() {
        for {
            // Get list of all containers
            cmdList := exec.Command("sudo", "docker", "ps", "-a", "--format", "{{.ID}}:{{.Names}}:{{.Status}}")
            outputList, err := cmdList.Output()
            if err != nil {
                fmt.Println("Error executing command:", err)
                continue
            }

            // Split output into lines
            linesList := strings.Split(string(outputList), "\n")
            for _, line := range linesList {
                line = strings.TrimSpace(line)
                if line == "" {
                    continue
                }
                parts := strings.Split(line, ":")
                if len(parts) < 3 {
                    continue
                }

                containerID := strings.TrimSpace(parts[0])
                containerName := strings.TrimSpace(parts[1])
                containerStatusStr := parts[2]

                // Determine if container is running
                status := 0.0
                if strings.Contains(containerStatusStr, "Up") {
                    status = 1.0
                }

                // Update status metric
                containerStatus.WithLabelValues(containerName, containerID).Set(status)

                if status == 1.0 {
                    // Only collect stats for running containers
                    cmdStats := exec.Command("sudo", "docker", "stats", "--no-stream", "--format", "{{.ID}}:{{.Name}}:{{.CPUPerc}}:{{.MemPerc}}:{{.NetIO}}")
                    outputStats, err := cmdStats.Output()
                    if err != nil {
                        fmt.Println("Error executing command:", err)
                        continue
                    }

                    // Split output into lines
                    linesStats := strings.Split(string(outputStats), "\n")
                    for _, line := range linesStats {
                        line = strings.TrimSpace(line)
                        if line == "" {
                            continue
                        }
                        parts := strings.Split(line, ":")
                        if len(parts) < 5 {
                            continue
                        }

                        id := strings.TrimSpace(parts[0])
                        name := strings.TrimSpace(parts[1])
                        cpuPercentStr := strings.TrimSpace(strings.TrimSuffix(parts[2], "%"))
                        memPercentStr := strings.TrimSpace(strings.TrimSuffix(parts[3], "%"))
                        netIO := strings.Split(strings.TrimSpace(parts[4]), " / ")

                        cpuPercent, err := strconv.ParseFloat(cpuPercentStr, 64)
                        if err != nil {
                            fmt.Println("Error parsing CPU output:", err)
                            continue
                        }
                        memPercent, err := strconv.ParseFloat(memPercentStr, 64)
                        if err != nil {
                            fmt.Println("Error parsing Memory output:", err)
                            continue
                        }

                        // Network IO Parsing
                        netReceivedBytes := parseBytes(netIO[0])
                        netTransmittedBytes := parseBytes(netIO[1])

                        // Update Prometheus metrics
                        containerCPUUsage.WithLabelValues(name, id).Set(cpuPercent)
                        containerMemoryUsage.WithLabelValues(name, id).Set(memPercent)
                        containerNetworkReceived.WithLabelValues(name, id).Set(netReceivedBytes)
                        containerNetworkTransmitted.WithLabelValues(name, id).Set(netTransmittedBytes)
                    }
                } else {
                    // If the container is stopped, reset the metrics to 0
                    containerCPUUsage.WithLabelValues(containerName, containerID).Set(0)
                    containerMemoryUsage.WithLabelValues(containerName, containerID).Set(0)
                    containerNetworkReceived.WithLabelValues(containerName, containerID).Set(0)
                    containerNetworkTransmitted.WithLabelValues(containerName, containerID).Set(0)
                }
            }

            time.Sleep(5 * time.Second) // Adjust as needed
        }
    }()
}

func parseBytes(size string) float64 {
    if strings.HasSuffix(size, "kB") {
        size = strings.TrimSuffix(size, "kB")
        value, _ := strconv.ParseFloat(size, 64)
        return value * 1024
    } else if strings.HasSuffix(size, "MB") {
        size = strings.TrimSuffix(size, "MB")
        value, _ := strconv.ParseFloat(size, 64)
        return value * 1024 * 1024
    } else if strings.HasSuffix(size, "GB") {
        size = strings.TrimSuffix(size, "GB")
        value, _ := strconv.ParseFloat(size, 64)
        return value * 1024 * 1024 * 1024
    }
    value, _ := strconv.ParseFloat(size, 64)
    return value
}

func main() {
    recordMetrics()

    http.Handle("/", promhttp.Handler())
    fmt.Println("Starting server at :1919")
    http.ListenAndServe(":1919", nil)
}


