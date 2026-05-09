package dev.pushkar.orchestrator;

import io.fabric8.kubernetes.api.model.apps.Deployment;
import io.fabric8.kubernetes.api.model.apps.DeploymentBuilder;
import io.fabric8.kubernetes.api.model.apps.ReplicaSet;
import io.fabric8.kubernetes.client.KubernetesClient;
import io.fabric8.kubernetes.client.KubernetesClientBuilder;
import io.fabric8.kubernetes.client.Watch;
import io.fabric8.kubernetes.client.Watcher;
import io.fabric8.kubernetes.client.WatcherException;

import java.util.List;
import java.util.Map;

/**
 * OrchestratorClient shows how the Fabric8 Kubernetes Java client performs
 * the same operations that our Go orchestrator implements from scratch.
 *
 * <p><b>Mapping from Go lab to Fabric8:</b>
 * <pre>
 *   Go lab                        Fabric8 / Kubernetes API
 *   ──────────────────────────    ──────────────────────────────────────────
 *   Deployment struct             io.fabric8.kubernetes.api.model.apps.Deployment
 *   DeploymentController.Apply()  client.apps().deployments().create()
 *   RollingUpdate strategy        spec.strategy.rollingUpdate (maxSurge, maxUnavailable)
 *   ReplicaSet                    io.fabric8.kubernetes.api.model.apps.ReplicaSet
 *   Store.Watch()                 client.apps().deployments().watch(watcher)
 *   WorkQueue.Enqueue()           controller-runtime's workqueue.RateLimitingInterface
 *   Reconciler.Reconcile()        controller-runtime's Reconciler interface
 * </pre>
 *
 * <p>Keep this class under 60 lines of logic; Spring wiring is in
 * {@link OrchestratorAutoConfiguration}.
 */
public class OrchestratorClient {

    private final OrchestratorProperties props;

    public OrchestratorClient(OrchestratorProperties props) {
        this.props = props;
    }

    /**
     * Creates a Deployment in Kubernetes with 3 replicas and a RollingUpdate
     * strategy — the same operation our Go DeploymentController.Apply() performs.
     */
    public void createDeployment(String name, String image, int replicas) {
        try (KubernetesClient client = new KubernetesClientBuilder().build()) {
            Deployment deployment = new DeploymentBuilder()
                .withNewMetadata()
                    .withName(name)
                    .withNamespace(props.getNamespace())
                    .withLabels(Map.of("app", name))
                .endMetadata()
                .withNewSpec()
                    .withReplicas(replicas)
                    .withNewSelector()
                        .addToMatchLabels("app", name)
                    .endSelector()
                    .withNewStrategy()
                        .withType("RollingUpdate")
                        .withNewRollingUpdate()
                            .withNewMaxSurge(1)
                            .withNewMaxUnavailable(1)
                        .endRollingUpdate()
                    .endStrategy()
                    .withNewTemplate()
                        .withNewMetadata()
                            .addToLabels("app", name)
                        .endMetadata()
                        .withNewSpec()
                            .addNewContainer()
                                .withName(name)
                                .withImage(image)
                            .endContainer()
                        .endSpec()
                    .endTemplate()
                .endSpec()
                .build();

            client.apps().deployments()
                .inNamespace(props.getNamespace())
                .resource(deployment)
                .create();
        }
    }

    /**
     * Lists all ReplicaSets owned by a Deployment — equivalent to reading
     * DeploymentController.ReplicaSet(name) in our Go lab.
     */
    public List<ReplicaSet> listReplicaSets(String deploymentName) {
        try (KubernetesClient client = new KubernetesClientBuilder().build()) {
            return client.apps().replicaSets()
                .inNamespace(props.getNamespace())
                .withLabel("app", deploymentName)
                .list()
                .getItems();
        }
    }

    /**
     * Returns a description of the reconcile loop relationship between
     * our Go lab and production Kubernetes controllers.
     */
    public String describeReconcileLoop() {
        return """
            Our Go Reconciler.Reconcile() maps to the controller-runtime Reconciler interface:

              type Reconciler interface {
                  Reconcile(ctx context.Context, req Request) (Result, error)
              }

            Key differences from production:
            - Production controllers use rate-limited WorkQueues (exponential backoff on error)
            - Production informers use List+Watch (initial list syncs the cache, then Watch streams deltas)
            - Production Store uses etcd for persistence; our Store is in-memory
            - Production controllers requeue with Result{RequeueAfter: 30s} for periodic self-healing
            - Our ControlLoop ticker achieves the same effect at the cost of predictability
            """;
    }
}
