import { useParams } from 'react-router-dom';
import { ApprovalLoadingSkeleton } from '@components/approvals/ApprovalLoadingSkeleton';
import { ApprovalErrorBanner } from '@components/approvals/ApprovalErrorBanner';
import { ApprovalReviewPage } from '@components/approvals/ApprovalReviewPage';
import { useApprovalReview } from '@hooks/useApprovalReview';
import { usePrincipal } from '@services/query/QueryProvider';

export function ApprovalPage() {
  const { id } = useParams<{ id: string }>();
  if (!id) return <ApprovalErrorBanner errorCode="NOT_FOUND" />;
  return <ApprovalPageContent key={id} approvalId={id} />;
}

function ApprovalPageContent({ approvalId }: { approvalId: string }) {
  const { principal } = usePrincipal();
  const { approval, loading, submitting, submittingAction, errorCode, errorMessage, approveResult, denyResult, approve, deny, onPreview, refetch } = useApprovalReview(approvalId);
  if (loading) return <ApprovalLoadingSkeleton />;
  if (!approval) return <ApprovalErrorBanner errorCode={errorCode ?? 'NOT_FOUND'} message={errorMessage} onRetry={refetch} />;
  return <ApprovalReviewPage approval={approval} actingPrincipal={principal} submitting={submitting} submittingAction={submittingAction} errorCode={errorCode} errorMessage={errorMessage} approveResult={approveResult} denyResult={denyResult} onApprove={approve} onDeny={deny} onPreview={onPreview} onRetry={refetch} />;
}

export default ApprovalPage;
