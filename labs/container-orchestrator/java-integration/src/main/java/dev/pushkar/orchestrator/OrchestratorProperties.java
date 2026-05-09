package dev.pushkar.orchestrator;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the OrchestratorClient demo.
 *
 * <p>Bind via {@code application.properties}:
 * <pre>
 *   orchestrator.namespace=default
 *   orchestrator.kubeconfig-path=~/.kube/config
 * </pre>
 */
@ConfigurationProperties(prefix = "orchestrator")
public class OrchestratorProperties {

    /** Kubernetes namespace to operate in. Defaults to "default". */
    private String namespace = "default";

    /** Path to kubeconfig file. If blank, Fabric8 uses in-cluster config. */
    private String kubeconfigPath = "";

    public String getNamespace() {
        return namespace;
    }

    public void setNamespace(String namespace) {
        this.namespace = namespace;
    }

    public String getKubeconfigPath() {
        return kubeconfigPath;
    }

    public void setKubeconfigPath(String kubeconfigPath) {
        this.kubeconfigPath = kubeconfigPath;
    }
}
