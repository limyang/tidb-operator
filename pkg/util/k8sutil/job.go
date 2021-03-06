package k8sutil

import (
	"github.com/astaxie/beego/logs"
	"github.com/ffan/tidb-operator/pkg/util/retryutil"

	"time"

	"k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateAndWaitJob create and wait job status 'running'
func CreateAndWaitJob(job *v1.Job, timeout time.Duration) (*v1.Job, error) {
	retjob, err := kubecli.BatchV1().Jobs(Namespace).Create(job)
	if err != nil {
		return nil, err
	}

	interval := time.Second
	err = retryutil.Retry(interval, int(timeout/(interval)), func() (bool, error) {
		retjob, err = kubecli.BatchV1().Jobs(Namespace).Get(job.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch retjob.Status.Active {
		case 1:
			return true, nil
		default:
			return false, nil
		}
	})
	logs.Info("Job '%s' created", retjob.GetName())
	return retjob, err
}

// DeleteJob delete a job by name
func DeleteJob(name string) error {
	err := kubecli.BatchV1().Jobs(Namespace).Delete(name, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	err = DeletePodsByLabel(map[string]string{"job-name": name})
	if err != nil {
		return err
	}
	return nil
}

// GetJob get a job by name
func GetJob(name string) (*v1.Job, error) {
	return kubecli.BatchV1().Jobs(Namespace).Get(name, metav1.GetOptions{})
}
