export interface Contact {
  /** ID either in lid or jid format (preferred) **/
  id: string;
  /** ID in LID format (@lid) **/
  lid?: string;
  /** ID in PN format (@s.whatsapp.net)  **/
  phoneNumber?: string;
  /** name of the contact, you have saved on your WA */
  name?: string;
  /** name of the contact, the contact has set on their own on WA */
  notify?: string;
  /** I have no idea */
  verifiedName?: string;
  imgUrl?: string | null;
  status?: string;
}
