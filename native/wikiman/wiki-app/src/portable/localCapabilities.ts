/** Local Wiki features do not require a WikiMan account or a paid subscription.
 * Official cloud endpoints are unavailable in the portable kernel.
 */
export const needSubscribe = () => false;
export const isPaidUser = () => true;
