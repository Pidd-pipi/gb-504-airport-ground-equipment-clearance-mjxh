import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable, map } from 'rxjs';
import { ApiResponse, ClearanceDecision, ClearanceState, ClearanceSummary, PageResult, ReconsiderationEligibility } from '../types';
import { API_BASE, extractData } from '../utils/request';

export function clearanceListApi(http: HttpClient, page = 1, pageSize = 50, state = ''): Observable<PageResult<ClearanceDecision>> {
  let params = new HttpParams().set('page', page).set('page_size', pageSize);
  if (state) params = params.set('state', state);
  return http.get<ApiResponse<PageResult<ClearanceDecision>>>(`${API_BASE}/v1/clearance`, { params }).pipe(map(extractData));
}

export function clearanceSummaryApi(http: HttpClient): Observable<ClearanceSummary> {
  return http.get<ApiResponse<ClearanceSummary>>(`${API_BASE}/v1/clearance/summary`).pipe(map(extractData));
}

export interface ClearancePayload {
  turnaround_id: number;
  state: Exclude<ClearanceState, 'pending'>;
  restrictions: string;
  reason: string;
  evidence: string[];
}

export function clearanceDecideApi(http: HttpClient, payload: ClearancePayload): Observable<ClearanceDecision> {
  return http.post<ApiResponse<ClearanceDecision>>(`${API_BASE}/v1/clearance`, payload).pipe(map(extractData));
}

export function reconsiderationEligibilityApi(http: HttpClient, turnaroundId: number): Observable<ReconsiderationEligibility> {
  const params = new HttpParams().set('turnaround_id', turnaroundId);
  return http.get<ApiResponse<ReconsiderationEligibility>>(`${API_BASE}/v1/clearance/reconsideration/eligibility`, { params }).pipe(map(extractData));
}

export interface ReconsiderationPayload {
  turnaround_id: number;
  reason: string;
  evidence: string[];
}

export function clearanceReconsiderApi(http: HttpClient, payload: ReconsiderationPayload): Observable<ClearanceDecision> {
  return http.post<ApiResponse<ClearanceDecision>>(`${API_BASE}/v1/clearance/reconsideration`, payload).pipe(map(extractData));
}
